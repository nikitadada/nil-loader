package auth

import (
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"net/http"
	"os"
	"strings"
	"time"
)

const (
	CookieName = "nil_loader_auth"
	// DevDefaultSecret is used only when NIL_LOADER_AUTH_SECRET is not set.
	// It's intentionally static so the app works out of the box.
	// For any real usage you should set NIL_LOADER_AUTH_SECRET.
	DevDefaultSecret = "nil-loader-dev-secret-change-me"

	PasswordMacOsWindows = "MacOs>Windows"
	PasswordMerlion      = "merlion"
)

type Service struct {
	secret     []byte
	ttl        time.Duration
	cookieName string
	passwords  map[string]struct{}
}

func NewService(secret []byte, ttl time.Duration) *Service {
	if len(secret) == 0 {
		secret = []byte(DevDefaultSecret)
	}
	pw := map[string]struct{}{
		PasswordMacOsWindows: {},
		PasswordMerlion:      {},
	}
	return &Service{
		secret:     append([]byte(nil), secret...),
		ttl:        ttl,
		cookieName: CookieName,
		passwords:  pw,
	}
}

func NewServiceFromEnv(ttl time.Duration) *Service {
	secret := os.Getenv("NIL_LOADER_AUTH_SECRET")
	if strings.TrimSpace(secret) == "" {
		return NewService([]byte(DevDefaultSecret), ttl)
	}
	return NewService([]byte(secret), ttl)
}

func (s *Service) CheckPassword(password string) bool {
	_, ok := s.passwords[password]
	return ok
}

func (s *Service) IsAuthenticatedRequest(r *http.Request) bool {
	c, err := r.Cookie(s.cookieName)
	if err != nil {
		return false
	}
	return s.validateSessionValue(c.Value)
}

func (s *Service) NewSessionValue(now time.Time) (string, error) {
	exp := now.Add(s.ttl).Unix()

	nonceBytes := make([]byte, 16)
	if _, err := rand.Read(nonceBytes); err != nil {
		return "", fmt.Errorf("generate nonce: %w", err)
	}
	nonce := hex.EncodeToString(nonceBytes)

	payload := fmt.Sprintf("%d|%s", exp, nonce)
	mac := hmac.New(sha256.New, s.secret)
	_, _ = mac.Write([]byte(payload))
	sig := hex.EncodeToString(mac.Sum(nil))

	return fmt.Sprintf("%s|%s", payload, sig), nil
}

func (s *Service) validateSessionValue(value string) bool {
	parts := strings.Split(value, "|")
	if len(parts) != 3 {
		return false
	}
	expStr, nonce, sigHex := parts[0], parts[1], parts[2]
	if expStr == "" || nonce == "" || sigHex == "" {
		return false
	}

	expUnix, err := parseUnixSeconds(expStr)
	if err != nil {
		return false
	}
	if time.Now().Unix() >= expUnix {
		return false
	}

	payload := fmt.Sprintf("%s|%s", expStr, nonce)
	expectedSig, err := computeSigHex(s.secret, payload)
	if err != nil {
		return false
	}

	expectedSigBytes, err := hex.DecodeString(expectedSig)
	if err != nil {
		return false
	}
	gotSigBytes, err := hex.DecodeString(sigHex)
	if err != nil {
		return false
	}
	if len(expectedSigBytes) != len(gotSigBytes) {
		return false
	}
	return hmac.Equal(expectedSigBytes, gotSigBytes)
}

func (s *Service) SetAuthCookie(w http.ResponseWriter, r *http.Request, now time.Time) error {
	value, err := s.NewSessionValue(now)
	if err != nil {
		return err
	}

	secure := r.TLS != nil
	expires := now.Add(s.ttl)

	// Signed cookie is the only gate, so keep it HttpOnly.
	http.SetCookie(w, &http.Cookie{
		Name:     s.cookieName,
		Value:    value,
		Path:     "/",
		Expires:  expires,
		MaxAge:   int(s.ttl.Seconds()),
		HttpOnly: true,
		Secure:   secure,
		SameSite: http.SameSiteLaxMode,
	})

	return nil
}

func parseUnixSeconds(s string) (int64, error) {
	var v int64
	_, err := fmt.Sscanf(s, "%d", &v)
	if err != nil {
		return 0, fmt.Errorf("parse exp: %w", err)
	}
	if v <= 0 {
		return 0, errors.New("exp must be positive")
	}
	return v, nil
}

func computeSigHex(secret []byte, payload string) (string, error) {
	if len(secret) == 0 {
		return "", errors.New("empty secret")
	}
	mac := hmac.New(sha256.New, secret)
	_, _ = mac.Write([]byte(payload))
	return hex.EncodeToString(mac.Sum(nil)), nil
}
