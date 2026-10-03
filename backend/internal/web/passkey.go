package web

import (
	"bytes"
	"crypto/hmac"
	"encoding/base64"
	"encoding/json"
	"errors"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"time"
	"unicode/utf8"

	"github.com/go-webauthn/webauthn/protocol"
	"github.com/go-webauthn/webauthn/webauthn"
	"github.com/gofiber/fiber/v2"

	"github.com/michaelputong/blog/backend/internal/auth"
)

// Passkey sign-in (WebAuthn). Each ceremony has two steps: "begin" returns
// the options for navigator.credentials, and "finish" verifies the browser's
// answer. The challenge between them travels in a short-lived cookie signed
// with the admin's session key; the server only remembers which challenges
// were used, so each works once.

const (
	challengeCookie = "passkey_challenge"
	challengeTTL    = 5 * time.Minute
)

// webAuthn configures the library for the address the admin is using: a
// passkey belongs to one site (its "relying party"), and the browser checks
// the page's origin against it.
func (h *pages) webAuthn(c *fiber.Ctx) (*webauthn.WebAuthn, error) {
	origin := c.BaseURL()
	u, err := url.Parse(origin)
	if err != nil {
		return nil, err
	}
	return webauthn.New(&webauthn.Config{
		RPID:          u.Hostname(),
		RPDisplayName: "GoBlog.dev Admin",
		RPOrigins:     []string{origin},
	})
}

// setChallenge stores the ceremony's session data for its finish step.
// purpose ("login" or "register") keeps one kind from being used for the other.
func setChallenge(c *fiber.Ctx, cred auth.Credential, purpose string, session *webauthn.SessionData) error {
	data, err := json.Marshal(session)
	if err != nil {
		return err
	}
	expires := time.Now().Add(challengeTTL)
	payload := purpose + "." + strconv.FormatInt(expires.Unix(), 10) + "." + base64.RawURLEncoding.EncodeToString(data)
	c.Cookie(&fiber.Cookie{
		Name:     challengeCookie,
		Value:    payload + "." + sign(cred.SessionKey, payload),
		Path:     "/admin",
		Expires:  expires,
		HTTPOnly: true,
		Secure:   c.Protocol() == "https",
		SameSite: fiber.CookieSameSiteStrictMode,
	})
	return nil
}

var errChallenge = errors.New("passkey challenge missing, expired, used or invalid")

// usedChallenges remembers challenges already answered until they expire, so
// a captured cookie and answer can't be replayed. It's per process: the app
// runs as a single machine.
type usedChallenges struct {
	mu   sync.Mutex
	seen map[string]time.Time // challenge → when it expires
}

// claim reports whether challenge is fresh, marking it used.
func (u *usedChallenges) claim(challenge string, expires time.Time) bool {
	u.mu.Lock()
	defer u.mu.Unlock()
	now := time.Now()
	if u.seen == nil {
		u.seen = map[string]time.Time{}
	}
	for ch, exp := range u.seen { // forget expired ones; their cookies are refused anyway
		if !now.Before(exp) {
			delete(u.seen, ch)
		}
	}
	if _, used := u.seen[challenge]; used {
		return false
	}
	u.seen[challenge] = expires
	return true
}

// takeChallenge reads and clears the ceremony's session data; each challenge
// can be taken once.
func (h *pages) takeChallenge(c *fiber.Ctx, cred auth.Credential, purpose string) (webauthn.SessionData, error) {
	var session webauthn.SessionData
	value := c.Cookies(challengeCookie)
	c.ClearCookie(challengeCookie) // one attempt per challenge
	i := strings.LastIndexByte(value, '.')
	if i < 0 {
		return session, errChallenge
	}
	payload, sig := value[:i], value[i+1:]
	if !hmac.Equal([]byte(sig), []byte(sign(cred.SessionKey, payload))) {
		return session, errChallenge
	}
	parts := strings.SplitN(payload, ".", 3)
	if len(parts) != 3 || parts[0] != purpose {
		return session, errChallenge
	}
	exp, err := strconv.ParseInt(parts[1], 10, 64)
	if err != nil || time.Now().Unix() >= exp {
		return session, errChallenge
	}
	data, err := base64.RawURLEncoding.DecodeString(parts[2])
	if err != nil || json.Unmarshal(data, &session) != nil || session.Challenge == "" {
		return session, errChallenge
	}
	if !h.challenges.claim(session.Challenge, time.Unix(exp, 0)) {
		return session, errChallenge
	}
	return session, nil
}

func jsonError(c *fiber.Ctx, status int, msg string) error {
	return c.Status(status).JSON(fiber.Map{"error": msg})
}

// ---- Sign in ------------------------------------------------------------

func (h *pages) passkeyLoginBegin(c *fiber.Ctx) error {
	cred, ok, err := h.credential(c)
	if err != nil {
		return h.fail(c, err)
	}
	if !ok || len(cred.Passkeys) == 0 {
		return jsonError(c, fiber.StatusBadRequest, "No passkeys are set up yet. Sign in with your password, then add one under Password.")
	}
	wa, err := h.webAuthn(c)
	if err != nil {
		return h.fail(c, err)
	}
	// Discoverable: the device offers the admin's passkey without a username.
	assertion, session, err := wa.BeginDiscoverableLogin(webauthn.WithUserVerification(protocol.VerificationRequired))
	if err != nil {
		return h.fail(c, err)
	}
	if err := setChallenge(c, cred, "login", session); err != nil {
		return h.fail(c, err)
	}
	return c.JSON(assertion)
}

func (h *pages) passkeyLoginFinish(c *fiber.Ctx) error {
	cred, ok, err := h.credential(c)
	if err != nil {
		return h.fail(c, err)
	}
	if !ok {
		return jsonError(c, fiber.StatusBadRequest, "No admin account exists yet.")
	}
	session, err := h.takeChallenge(c, cred, "login")
	if err != nil {
		return jsonError(c, fiber.StatusBadRequest, "This sign-in attempt expired. Try again.")
	}
	parsed, err := protocol.ParseCredentialRequestResponseBytes(c.Body())
	if err != nil {
		return jsonError(c, fiber.StatusBadRequest, "The browser's passkey response couldn't be read. Try again.")
	}
	wa, err := h.webAuthn(c)
	if err != nil {
		return h.fail(c, err)
	}
	owner := func(_, userHandle []byte) (webauthn.User, error) {
		if len(cred.UserID) == 0 || !bytes.Equal(userHandle, cred.UserID) {
			return nil, errors.New("passkey belongs to another account")
		}
		return auth.WebAuthnUser{Credential: cred}, nil
	}
	_, record, err := wa.ValidatePasskeyLogin(owner, session, parsed)
	if err != nil {
		return jsonError(c, fiber.StatusUnauthorized, "That passkey isn't registered for this site, or it couldn't be verified.")
	}
	updated, err := cred.WithPasskeyUsed(*record, time.Now())
	if err != nil {
		return h.fail(c, err)
	}
	if err := h.cfg.Credentials.Update(c.UserContext(), updated); err != nil {
		return h.fail(c, err)
	}
	h.startSession(c, updated)
	return c.JSON(fiber.Map{"redirect": "/admin"})
}

// ---- Manage (signed in) -------------------------------------------------

func (h *pages) passkeyRegisterBegin(c *fiber.Ctx) error {
	cred, _, err := h.credential(c)
	if err != nil {
		return h.fail(c, err)
	}
	if len(cred.Passkeys) >= auth.MaxPasskeys {
		return jsonError(c, fiber.StatusBadRequest, "You've reached the limit of "+strconv.Itoa(auth.MaxPasskeys)+" passkeys. Remove one first.")
	}
	if len(cred.UserID) == 0 { // the first passkey: create the user handle they're bound to
		if cred, err = cred.WithUserID(); err != nil {
			return h.fail(c, err)
		}
		if err := h.cfg.Credentials.Update(c.UserContext(), cred); err != nil {
			return h.fail(c, err)
		}
	}
	wa, err := h.webAuthn(c)
	if err != nil {
		return h.fail(c, err)
	}
	user := auth.WebAuthnUser{Credential: cred}
	existing := user.WebAuthnCredentials()
	exclude := make([]protocol.CredentialDescriptor, len(existing))
	for i := range existing {
		exclude[i] = existing[i].Descriptor() // don't register the same authenticator twice
	}
	creation, session, err := wa.BeginRegistration(user,
		webauthn.WithAuthenticatorSelection(protocol.AuthenticatorSelection{
			ResidentKey:        protocol.ResidentKeyRequirementRequired, // a passkey: usable without a username
			RequireResidentKey: protocol.ResidentKeyRequired(),
			UserVerification:   protocol.VerificationRequired,
		}),
		webauthn.WithExclusions(exclude),
	)
	if err != nil {
		return h.fail(c, err)
	}
	if err := setChallenge(c, cred, "register", session); err != nil {
		return h.fail(c, err)
	}
	return c.JSON(creation)
}

// passkeyRegisterFinish expects {"name": "...", "credential": <the browser's response>}.
func (h *pages) passkeyRegisterFinish(c *fiber.Ctx) error {
	cred, _, err := h.credential(c)
	if err != nil {
		return h.fail(c, err)
	}
	session, err := h.takeChallenge(c, cred, "register")
	if err != nil {
		return jsonError(c, fiber.StatusBadRequest, "Adding the passkey took too long. Try again.")
	}
	var body struct {
		Name       string          `json:"name"`
		Credential json.RawMessage `json:"credential"`
	}
	if err := json.Unmarshal(c.Body(), &body); err != nil {
		return jsonError(c, fiber.StatusBadRequest, "The browser's passkey response couldn't be read. Try again.")
	}
	name := strings.TrimSpace(body.Name)
	if name == "" {
		name = "Passkey"
	}
	if utf8.RuneCountInString(name) > 60 {
		return jsonError(c, fiber.StatusBadRequest, "Keep the passkey name under 60 characters.")
	}
	parsed, err := protocol.ParseCredentialCreationResponseBytes(body.Credential)
	if err != nil {
		return jsonError(c, fiber.StatusBadRequest, "The browser's passkey response couldn't be read. Try again.")
	}
	wa, err := h.webAuthn(c)
	if err != nil {
		return h.fail(c, err)
	}
	record, err := wa.CreateCredential(auth.WebAuthnUser{Credential: cred}, session, parsed)
	if err != nil {
		return jsonError(c, fiber.StatusBadRequest, "The passkey couldn't be verified. Try again.")
	}
	p, err := auth.NewPasskey(name, *record, time.Now())
	if err != nil {
		return h.fail(c, err)
	}
	if err := h.cfg.Credentials.Update(c.UserContext(), cred.WithPasskey(p)); err != nil {
		return h.fail(c, err)
	}
	return c.JSON(fiber.Map{"ok": true})
}

func (h *pages) passkeyDelete(c *fiber.Ctx) error {
	cred, _, err := h.credential(c)
	if err != nil {
		return h.fail(c, err)
	}
	updated, removed := cred.WithoutPasskey(c.Params("id"))
	if removed {
		if err := h.cfg.Credentials.Update(c.UserContext(), updated); err != nil {
			return h.fail(c, err)
		}
	}
	return c.Redirect("/admin/password?notice=passkey-removed", fiber.StatusSeeOther)
}

// passkeyView is a passkey as the Password page lists it.
type passkeyView struct {
	ID         string `json:"id"`
	Name       string `json:"name"`
	CreatedAt  string `json:"createdAt"`
	LastUsedAt string `json:"lastUsedAt"` // "" if never used
}

func passkeyViews(cred auth.Credential) []passkeyView {
	out := make([]passkeyView, 0, len(cred.Passkeys))
	for _, p := range cred.Passkeys {
		v := passkeyView{ID: p.ID(), Name: p.Name, CreatedAt: p.CreatedAt.UTC().Format(time.RFC3339)}
		if !p.LastUsedAt.IsZero() {
			v.LastUsedAt = p.LastUsedAt.UTC().Format(time.RFC3339)
		}
		out = append(out, v)
	}
	return out
}
