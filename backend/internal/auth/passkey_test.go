package auth

import (
	"encoding/base64"
	"testing"
	"time"

	"github.com/go-webauthn/webauthn/webauthn"
)

func TestPasskeys(t *testing.T) {
	c, err := NewCredential("Correct-Horse-7-Staple", 4)
	if err != nil {
		t.Fatal(err)
	}
	c, err = c.WithUserID()
	if err != nil || len(c.UserID) != 32 {
		t.Fatalf("WithUserID: %d bytes, %v", len(c.UserID), err)
	}
	if again, _ := c.WithUserID(); string(again.UserID) != string(c.UserID) {
		t.Error("WithUserID must keep an existing user handle: passkeys are bound to it")
	}

	day := time.Date(2026, 10, 3, 0, 0, 0, 0, time.UTC)
	phone, _ := NewPasskey("Phone", webauthn.Credential{ID: []byte{1, 2, 3}, PublicKey: []byte("pk1")}, day)
	laptop, _ := NewPasskey("Laptop", webauthn.Credential{ID: []byte{4, 5, 6}, PublicKey: []byte("pk2")}, day)
	c = c.WithPasskey(phone).WithPasskey(laptop)
	if got := (WebAuthnUser{c}).WebAuthnCredentials(); len(got) != 2 || string(got[1].PublicKey) != "pk2" {
		t.Fatalf("WebAuthnCredentials = %+v", got)
	}
	if phone.ID() != base64.RawURLEncoding.EncodeToString([]byte{1, 2, 3}) {
		t.Errorf("ID = %q", phone.ID())
	}

	// A sign-in updates that passkey's counter and last use, and nothing else.
	used := webauthn.Credential{ID: []byte{1, 2, 3}, PublicKey: []byte("pk1"), Authenticator: webauthn.Authenticator{SignCount: 7}}
	c2, err := c.WithPasskeyUsed(used, day.Add(time.Hour))
	if err != nil {
		t.Fatal(err)
	}
	if r, _ := c2.Passkeys[0].Record(); r.Authenticator.SignCount != 7 || !c2.Passkeys[0].LastUsedAt.Equal(day.Add(time.Hour)) {
		t.Errorf("used passkey = %+v", c2.Passkeys[0])
	}
	if !c2.Passkeys[1].LastUsedAt.IsZero() || !c.Passkeys[0].LastUsedAt.IsZero() {
		t.Error("WithPasskeyUsed changed another passkey, or the original credential")
	}

	c3, removed := c2.WithoutPasskey(phone.ID())
	if !removed || len(c3.Passkeys) != 1 || c3.Passkeys[0].Name != "Laptop" {
		t.Errorf("WithoutPasskey = %+v, %v", c3.Passkeys, removed)
	}
	if _, removed := c3.WithoutPasskey("unknown"); removed {
		t.Error("WithoutPasskey removed something for an unknown ID")
	}

	// Changing the password keeps the passkeys.
	if c4, _ := c3.WithPassword("Another-Strong-9-Pass", 4); len(c4.Passkeys) != 1 || string(c4.UserID) != string(c.UserID) {
		t.Error("WithPassword dropped the passkeys or the user handle")
	}
}
