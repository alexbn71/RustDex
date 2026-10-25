package server

import (
	"crypto/pbkdf2"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"fmt"
	"log"
	"net"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/alexbn71/RustDex/internal/store"
)

const pbkdf2Iterations = 600_000

// initAdmin prepares the admin password check. With RUSTDEX_ADMIN_PASSWORD
// set it is used as is; otherwise a random password is generated on first
// start, printed once in the log and stored as a PBKDF2 hash.
func (s *Server) initAdmin() error {
	if s.cfg.AdminPassword != "" {
		return nil
	}
	hash, err := s.st.GetMeta("admin_password_hash")
	if err == nil {
		s.adminHash = hash
		return nil
	}
	if !errors.Is(err, store.ErrNotFound) {
		return err
	}
	b := make([]byte, 12)
	if _, err := rand.Read(b); err != nil {
		return err
	}
	password := base64.RawURLEncoding.EncodeToString(b)
	if s.adminHash, err = hashPassword(password); err != nil {
		return err
	}
	if err := s.st.SetMeta("admin_password_hash", s.adminHash); err != nil {
		return err
	}
	log.Printf("generated password for user %q: %s (shown only once; set RUSTDEX_ADMIN_PASSWORD to choose your own)",
		s.cfg.AdminUser, password)
	return nil
}

func hashPassword(password string) (string, error) {
	salt := make([]byte, 16)
	if _, err := rand.Read(salt); err != nil {
		return "", err
	}
	key, err := pbkdf2.Key(sha256.New, password, salt, pbkdf2Iterations, 32)
	if err != nil {
		return "", err
	}
	return fmt.Sprintf("pbkdf2-sha256$%d$%s$%s", pbkdf2Iterations,
		base64.RawStdEncoding.EncodeToString(salt), base64.RawStdEncoding.EncodeToString(key)), nil
}

func verifyHash(encoded, password string) bool {
	parts := strings.Split(encoded, "$")
	if len(parts) != 4 || parts[0] != "pbkdf2-sha256" {
		return false
	}
	iter, err := strconv.Atoi(parts[1])
	if err != nil {
		return false
	}
	salt, err1 := base64.RawStdEncoding.DecodeString(parts[2])
	want, err2 := base64.RawStdEncoding.DecodeString(parts[3])
	if err1 != nil || err2 != nil {
		return false
	}
	got, err := pbkdf2.Key(sha256.New, password, salt, iter, len(want))
	return err == nil && subtle.ConstantTimeCompare(got, want) == 1
}

func (s *Server) checkCredentials(user, password string) bool {
	userOK := equalHashed(user, s.cfg.AdminUser)
	var passOK bool
	if s.cfg.AdminPassword != "" {
		passOK = equalHashed(password, s.cfg.AdminPassword)
	} else {
		passOK = verifyHash(s.adminHash, password)
	}
	return userOK && passOK
}

// equalHashed compares in constant time regardless of the lengths.
func equalHashed(a, b string) bool {
	ha, hb := sha256.Sum256([]byte(a)), sha256.Sum256([]byte(b))
	return subtle.ConstantTimeCompare(ha[:], hb[:]) == 1
}

func tokenHash(token string) string {
	h := sha256.Sum256([]byte(token))
	return hex.EncodeToString(h[:])
}

func (s *Server) newSession(client string) (string, error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	token := hex.EncodeToString(b)
	now := time.Now()
	s.st.PurgeSessions(now.Unix())
	err := s.st.CreateSession(tokenHash(token), client, now.Unix(), now.Add(s.cfg.SessionTTL).Unix())
	return token, err
}

func bearer(r *http.Request) string {
	h := r.Header.Get("Authorization")
	if len(h) > 7 && strings.EqualFold(h[:7], "bearer ") {
		return strings.TrimSpace(h[7:])
	}
	return ""
}

// auth rejects requests without a valid session token. Sessions slide: one
// used past half of its lifetime is extended.
func (s *Server) auth(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		token := bearer(r)
		if token == "" {
			writeError(w, http.StatusUnauthorized, "not logged in")
			return
		}
		th := tokenHash(token)
		exp, err := s.st.SessionExpiry(th)
		now := time.Now()
		if errors.Is(err, store.ErrNotFound) || (err == nil && exp < now.Unix()) {
			writeError(w, http.StatusUnauthorized, "session expired, log in again")
			return
		}
		if err != nil {
			internalError(w, err)
			return
		}
		if time.Unix(exp, 0).Sub(now) < s.cfg.SessionTTL/2 {
			s.st.ExtendSession(th, now.Add(s.cfg.SessionTTL).Unix())
		}
		next(w, r)
	}
}

// loginLimiter locks out an address for a while after repeated failures.
type loginLimiter struct {
	mu    sync.Mutex
	state map[string]*loginFails
}

type loginFails struct {
	count int
	until time.Time
	last  time.Time
}

const (
	maxLoginFails = 5
	lockoutTime   = 15 * time.Minute
)

func newLoginLimiter() *loginLimiter {
	return &loginLimiter{state: map[string]*loginFails{}}
}

// blocked returns how long the address is still locked out.
func (l *loginLimiter) blocked(ip string) time.Duration {
	l.mu.Lock()
	defer l.mu.Unlock()
	if f := l.state[ip]; f != nil {
		return time.Until(f.until)
	}
	return 0
}

func (l *loginLimiter) fail(ip string) {
	l.mu.Lock()
	defer l.mu.Unlock()
	now := time.Now()
	for k, f := range l.state {
		if now.Sub(f.last) > lockoutTime && now.After(f.until) {
			delete(l.state, k)
		}
	}
	f := l.state[ip]
	if f == nil {
		f = &loginFails{}
		l.state[ip] = f
	}
	f.count++
	f.last = now
	if f.count >= maxLoginFails {
		f.count = 0
		f.until = now.Add(lockoutTime)
	}
}

func (l *loginLimiter) success(ip string) {
	l.mu.Lock()
	defer l.mu.Unlock()
	delete(l.state, ip)
}

// clientIP returns the caller address; behind a reverse proxy
// (RUSTDEX_TRUST_PROXY) the one the proxy reports.
func (s *Server) clientIP(r *http.Request) string {
	if s.cfg.TrustProxy {
		if ip := strings.TrimSpace(r.Header.Get("X-Real-IP")); ip != "" {
			return ip
		}
		if xff := r.Header.Get("X-Forwarded-For"); xff != "" {
			parts := strings.Split(xff, ",")
			return strings.TrimSpace(parts[len(parts)-1])
		}
	}
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return r.RemoteAddr
	}
	return host
}
