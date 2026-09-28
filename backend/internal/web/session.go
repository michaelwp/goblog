package web

import (
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"fmt"
	"log"
	"math/big"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/michaelputong/blog/backend/internal/auth"
)

const sessionTTL = 12 * time.Hour

// Admin session tokens are "<unix expiry>.<epoch>.<HMAC>", signed with the
// key stored alongside the password. The epoch changes with the password,
// so a password change signs out every existing session.
func issueToken(c auth.Credential, now time.Time) (token string, expires time.Time) {
	expires = now.Add(sessionTTL)
	payload := strconv.FormatInt(expires.Unix(), 10) + "." + strconv.FormatInt(c.Epoch, 10)
	return payload + "." + sign(c.SessionKey, payload), expires
}

func validToken(token string, c auth.Credential, now time.Time) bool {
	i := strings.LastIndexByte(token, '.')
	if i < 0 {
		return false
	}
	payload, sig := token[:i], token[i+1:]
	if !hmac.Equal([]byte(sig), []byte(sign(c.SessionKey, payload))) {
		return false
	}
	expStr, epochStr, ok := strings.Cut(payload, ".")
	exp, err1 := strconv.ParseInt(expStr, 10, 64)
	epoch, err2 := strconv.ParseInt(epochStr, 10, 64)
	return ok && err1 == nil && err2 == nil && epoch == c.Epoch && now.Unix() < exp
}

func sign(key []byte, payload string) string {
	mac := hmac.New(sha256.New, key)
	mac.Write([]byte(payload))
	return base64.RawURLEncoding.EncodeToString(mac.Sum(nil))
}

// setupCodes guards first-time admin setup. Until an admin password exists,
// anyone who can reach /admin could claim the account, so setup also needs a
// one-time code that only the server's operator can see: it's printed to the
// server log.
type setupCodes struct {
	mu      sync.Mutex
	code    string
	expires time.Time
	now     func() time.Time
	logf    func(format string, args ...any)
}

const setupCodeTTL = time.Hour

// current returns the active code, generating and logging a new one if
// there is none or it expired.
func (s *setupCodes) current() string {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.code == "" || !s.now().Before(s.expires) {
		n, err := rand.Int(rand.Reader, big.NewInt(100_000_000))
		if err != nil {
			panic(err)
		}
		digits := fmt.Sprintf("%08d", n.Int64())
		s.code, s.expires = digits[:4]+"-"+digits[4:], s.now().Add(setupCodeTTL)
		logf := s.logf
		if logf == nil {
			logf = log.Printf
		}
		logf("Admin setup code: %s (valid for 1 hour). Enter it at /admin/setup to create the admin password.", s.code)
	}
	return s.code
}

func (s *setupCodes) matches(given string) bool {
	given = strings.ReplaceAll(strings.TrimSpace(given), " ", "")
	if len(given) == 8 && !strings.Contains(given, "-") {
		given = given[:4] + "-" + given[4:]
	}
	want := s.current()
	return subtle.ConstantTimeCompare([]byte(given), []byte(want)) == 1
}

// clear invalidates the code once setup is done.
func (s *setupCodes) clear() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.code = ""
}
