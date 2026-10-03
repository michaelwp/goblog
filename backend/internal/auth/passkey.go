package auth

import (
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"slices"
	"time"

	"github.com/go-webauthn/webauthn/webauthn"
)

// MaxPasskeys bounds how many passkeys the admin can register.
const MaxPasskeys = 20

// Passkey is a WebAuthn credential the admin can sign in with instead of the
// password. Data holds the library's credential record as JSON, so the stored
// form doesn't depend on its Go struct layout.
type Passkey struct {
	Name       string    `bson:"name"`
	Data       []byte    `bson:"data"` // JSON of webauthn.Credential
	CreatedAt  time.Time `bson:"createdAt"`
	LastUsedAt time.Time `bson:"lastUsedAt,omitempty"`
}

// Record decodes the WebAuthn credential record.
func (p Passkey) Record() (webauthn.Credential, error) {
	var c webauthn.Credential
	if err := json.Unmarshal(p.Data, &c); err != nil {
		return c, fmt.Errorf("decode passkey: %w", err)
	}
	return c, nil
}

// ID is the credential ID, base64url-encoded, as used in URLs.
func (p Passkey) ID() string {
	c, err := p.Record()
	if err != nil {
		return ""
	}
	return base64.RawURLEncoding.EncodeToString(c.ID)
}

// NewPasskey wraps a credential the library just verified.
func NewPasskey(name string, record webauthn.Credential, now time.Time) (Passkey, error) {
	data, err := json.Marshal(record)
	if err != nil {
		return Passkey{}, err
	}
	return Passkey{Name: name, Data: data, CreatedAt: now.UTC()}, nil
}

// WithUserID returns c with a WebAuthn user handle, creating a random one the
// first time. Passkeys are bound to it, so it never changes afterwards.
func (c Credential) WithUserID() (Credential, error) {
	if len(c.UserID) > 0 {
		return c, nil
	}
	id := make([]byte, 32)
	if _, err := rand.Read(id); err != nil {
		return c, err
	}
	c.UserID = id
	return c, nil
}

// WithPasskey returns c with p added.
func (c Credential) WithPasskey(p Passkey) Credential {
	c.Passkeys = append(slices.Clone(c.Passkeys), p)
	return c
}

// WithoutPasskey returns c without the passkey whose ID is id, and whether
// one was removed.
func (c Credential) WithoutPasskey(id string) (Credential, bool) {
	kept := make([]Passkey, 0, len(c.Passkeys))
	for _, p := range c.Passkeys {
		if p.ID() != id {
			kept = append(kept, p)
		}
	}
	removed := len(kept) < len(c.Passkeys)
	c.Passkeys = kept
	return c, removed
}

// WithPasskeyUsed records a sign-in with record: its updated signature
// counter and flags, and the time.
func (c Credential) WithPasskeyUsed(record webauthn.Credential, now time.Time) (Credential, error) {
	data, err := json.Marshal(record)
	if err != nil {
		return c, err
	}
	id := base64.RawURLEncoding.EncodeToString(record.ID)
	c.Passkeys = slices.Clone(c.Passkeys)
	for i, p := range c.Passkeys {
		if p.ID() == id {
			c.Passkeys[i].Data, c.Passkeys[i].LastUsedAt = data, now.UTC()
		}
	}
	return c, nil
}

// WebAuthnUser presents the admin to the WebAuthn library. There is a single
// admin account, so the name is fixed.
type WebAuthnUser struct{ Credential }

func (u WebAuthnUser) WebAuthnID() []byte          { return u.UserID }
func (u WebAuthnUser) WebAuthnName() string        { return "admin" }
func (u WebAuthnUser) WebAuthnDisplayName() string { return "Admin" }

// WebAuthnCredentials skips records that fail to decode rather than failing
// every sign-in because of one.
func (u WebAuthnUser) WebAuthnCredentials() []webauthn.Credential {
	out := make([]webauthn.Credential, 0, len(u.Passkeys))
	for _, p := range u.Passkeys {
		if c, err := p.Record(); err == nil {
			out = append(out, c)
		}
	}
	return out
}
