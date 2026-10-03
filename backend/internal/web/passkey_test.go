package web

import (
	"bytes"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/binary"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/fxamacker/cbor/v2"
	"github.com/gofiber/fiber/v2"
)

// softAuthenticator stands in for a phone or security key: it holds one
// P-256 key and answers WebAuthn ceremonies for the test server's origin.
type softAuthenticator struct {
	key    *ecdsa.PrivateKey
	credID []byte
	userID []byte // the user handle it was registered for
	count  uint32
}

const testOrigin = "http://example.com" // httptest requests use this host

func newSoftAuthenticator(t *testing.T) *softAuthenticator {
	t.Helper()
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	id := make([]byte, 16)
	_, _ = rand.Read(id)
	return &softAuthenticator{key: key, credID: id}
}

var b64 = base64.RawURLEncoding

func clientData(typ, challenge string) []byte {
	b, _ := json.Marshal(map[string]any{"type": typ, "challenge": challenge, "origin": testOrigin, "crossOrigin": false})
	return b
}

// authData builds authenticator data: RP ID hash, flags (user present and
// verified), the signature counter, and optionally the new credential.
func (a *softAuthenticator) authData(attested []byte) []byte {
	rp := sha256.Sum256([]byte("example.com"))
	flags := byte(0x01 | 0x04) // UP | UV
	if attested != nil {
		flags |= 0x40 // AT
	}
	a.count++
	out := append(rp[:], flags)
	out = binary.BigEndian.AppendUint32(out, a.count)
	return append(out, attested...)
}

func (a *softAuthenticator) register(t *testing.T, options []byte) map[string]any {
	t.Helper()
	var opts struct {
		PublicKey struct {
			Challenge string `json:"challenge"`
			User      struct {
				ID string `json:"id"`
			} `json:"user"`
		} `json:"publicKey"`
	}
	if err := json.Unmarshal(options, &opts); err != nil {
		t.Fatalf("registration options: %v: %s", err, options)
	}
	a.userID, _ = b64.DecodeString(opts.PublicKey.User.ID)

	cose, err := cbor.Marshal(map[int]any{
		1: 2, 3: -7, -1: 1, // EC2, ES256, P-256
		-2: a.key.X.FillBytes(make([]byte, 32)),
		-3: a.key.Y.FillBytes(make([]byte, 32)),
	})
	if err != nil {
		t.Fatal(err)
	}
	attested := make([]byte, 16) // AAGUID
	attested = binary.BigEndian.AppendUint16(attested, uint16(len(a.credID)))
	attested = append(append(attested, a.credID...), cose...)
	attObj, err := cbor.Marshal(map[string]any{"fmt": "none", "attStmt": map[string]any{}, "authData": a.authData(attested)})
	if err != nil {
		t.Fatal(err)
	}
	return map[string]any{
		"id": b64.EncodeToString(a.credID), "rawId": b64.EncodeToString(a.credID), "type": "public-key",
		"response": map[string]any{
			"clientDataJSON":    b64.EncodeToString(clientData("webauthn.create", opts.PublicKey.Challenge)),
			"attestationObject": b64.EncodeToString(attObj),
		},
	}
}

func (a *softAuthenticator) assert(t *testing.T, options []byte, userHandle []byte) map[string]any {
	t.Helper()
	var opts struct {
		PublicKey struct {
			Challenge string `json:"challenge"`
		} `json:"publicKey"`
	}
	if err := json.Unmarshal(options, &opts); err != nil {
		t.Fatalf("login options: %v: %s", err, options)
	}
	cd := clientData("webauthn.get", opts.PublicKey.Challenge)
	ad := a.authData(nil)
	hash := sha256.Sum256(cd)
	digest := sha256.Sum256(append(append([]byte{}, ad...), hash[:]...))
	sig, err := ecdsa.SignASN1(rand.Reader, a.key, digest[:])
	if err != nil {
		t.Fatal(err)
	}
	return map[string]any{
		"id": b64.EncodeToString(a.credID), "rawId": b64.EncodeToString(a.credID), "type": "public-key",
		"response": map[string]any{
			"clientDataJSON":    b64.EncodeToString(cd),
			"authenticatorData": b64.EncodeToString(ad),
			"signature":         b64.EncodeToString(sig),
			"userHandle":        b64.EncodeToString(userHandle),
		},
	}
}

type jsonResponse struct {
	code    int
	body    []byte
	cookies []*http.Cookie
}

func postJSON(t *testing.T, app *fiber.App, path string, body any, cookies ...*http.Cookie) jsonResponse {
	t.Helper()
	var r io.Reader
	if body != nil {
		b, _ := json.Marshal(body)
		r = bytes.NewReader(b)
	}
	req := httptest.NewRequest("POST", path, r)
	req.Header.Set("Content-Type", "application/json")
	for _, c := range cookies {
		if c != nil {
			req.AddCookie(c)
		}
	}
	resp, err := app.Test(req)
	if err != nil {
		t.Fatal(err)
	}
	b, _ := io.ReadAll(resp.Body)
	return jsonResponse{resp.StatusCode, b, resp.Cookies()}
}

func cookieNamed(cookies []*http.Cookie, name string) *http.Cookie {
	for _, c := range cookies {
		if c.Name == name && c.Value != "" {
			return c
		}
	}
	return nil
}

func TestPasskeyRegisterAndSignIn(t *testing.T) {
	app, _ := newAdminApp(t, testPassword)
	device := newSoftAuthenticator(t)

	// Before any passkey exists, sign-in says so.
	if res := postJSON(t, app, "/admin/passkey/login/begin", nil); res.code != 400 || !strings.Contains(string(res.body), "No passkeys") {
		t.Fatalf("begin without passkeys: %d %s", res.code, res.body)
	}
	// Adding one needs a signed-in admin.
	if res := postJSON(t, app, "/admin/passkeys/register/begin", nil); res.code != 303 {
		t.Fatalf("register while signed out: status %d, want redirect to login", res.code)
	}

	// Register a passkey while signed in with the password.
	session := login(t, app)
	begin := postJSON(t, app, "/admin/passkeys/register/begin", nil, session)
	if begin.code != 200 {
		t.Fatalf("register begin: %d %s", begin.code, begin.body)
	}
	challenge := cookieNamed(begin.cookies, challengeCookie)
	finish := postJSON(t, app, "/admin/passkeys/register/finish",
		map[string]any{"name": "Test phone", "credential": device.register(t, begin.body)}, session, challenge)
	if finish.code != 200 {
		t.Fatalf("register finish: %d %s", finish.code, finish.body)
	}
	if res := send(t, app, "GET", "/admin/password", nil, session); !strings.Contains(res.body, "Test phone") {
		t.Error("the Password page doesn't list the new passkey")
	}

	// Sign in with it, without a session.
	signIn := func() (jsonResponse, map[string]any, *http.Cookie) {
		begin := postJSON(t, app, "/admin/passkey/login/begin", nil)
		if begin.code != 200 {
			t.Fatalf("login begin: %d %s", begin.code, begin.body)
		}
		return begin, device.assert(t, begin.body, device.userID), cookieNamed(begin.cookies, challengeCookie)
	}
	_, assertion, challenge := signIn()
	res := postJSON(t, app, "/admin/passkey/login/finish", assertion, challenge)
	if res.code != 200 || !strings.Contains(string(res.body), `"/admin"`) {
		t.Fatalf("login finish: %d %s", res.code, res.body)
	}
	newSession := cookieNamed(res.cookies, sessionCookie)
	if newSession == nil {
		t.Fatal("passkey sign-in set no session cookie")
	}
	if page := send(t, app, "GET", "/admin", nil, newSession); page.code != 200 {
		t.Errorf("admin with the passkey session: status %d", page.code)
	}
	if page := send(t, app, "GET", "/admin/password", nil, newSession); !strings.Contains(page.body, "Last used") {
		t.Error("the passkey's last use isn't recorded")
	}

	// The same answer can't be replayed: the challenge is single-use.
	if replay := postJSON(t, app, "/admin/passkey/login/finish", assertion, challenge); replay.code < 400 || cookieNamed(replay.cookies, sessionCookie) != nil {
		t.Errorf("replayed sign-in: status %d", replay.code)
	}

	// A forged signature is rejected.
	_, forged, challenge := signIn()
	forged["response"].(map[string]any)["signature"] = b64.EncodeToString([]byte("not a signature"))
	if res := postJSON(t, app, "/admin/passkey/login/finish", forged, challenge); res.code != 401 {
		t.Errorf("forged signature: status %d, want 401", res.code)
	}

	// Removing the passkey stops it from working.
	id := b64.EncodeToString(device.credID)
	if res := send(t, app, "POST", "/admin/passkeys/"+id+"/delete", nil, newSession); res.code != 303 {
		t.Fatalf("delete: status %d", res.code)
	}
	if res := postJSON(t, app, "/admin/passkey/login/begin", nil); res.code != 400 {
		t.Errorf("begin after removing the only passkey: status %d, want 400", res.code)
	}
}

func TestPasskeyChallengeCookie(t *testing.T) {
	app, _ := newAdminApp(t, testPassword)
	session := login(t, app)
	device := newSoftAuthenticator(t)
	begin := postJSON(t, app, "/admin/passkeys/register/begin", nil, session)
	registration := device.register(t, begin.body)
	challenge := cookieNamed(begin.cookies, challengeCookie)

	// A tampered challenge cookie, or none at all, is refused.
	tampered := *challenge
	tampered.Value = strings.Replace(tampered.Value, "register.", "register.1", 1)
	for name, c := range map[string]*http.Cookie{"tampered": &tampered, "missing": nil} {
		res := postJSON(t, app, "/admin/passkeys/register/finish", map[string]any{"name": "x", "credential": registration}, session, c)
		if res.code != 400 || !strings.Contains(string(res.body), "took too long") {
			t.Errorf("%s challenge: %d %s", name, res.code, res.body)
		}
	}
	// A registration challenge can't be used to sign in.
	if res := postJSON(t, app, "/admin/passkey/login/finish", device.assert(t, begin.body, device.userID), challenge); res.code != 400 {
		t.Errorf("registration challenge used for sign-in: status %d, want 400", res.code)
	}
}
