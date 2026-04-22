// Package session implements HMAC-signed session cookies carrying a Telegram user
// ID. The cookie format is base64url(json({sub, exp})) + "." + base64url(hmac).
package session

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"time"
)

const (
	CookieName    = "oab_session"
	defaultMaxAge = 30 * 24 * time.Hour
)

var (
	ErrNoCookie     = errors.New("no session cookie")
	ErrBadSignature = errors.New("session signature invalid")
	ErrExpired      = errors.New("session expired")
	ErrMalformed    = errors.New("session malformed")
)

// Claims is the payload signed inside a session cookie.
type Claims struct {
	TelegramUserID int64 `json:"sub"`
	ExpiresAt      int64 `json:"exp"`
}

// Manager issues and verifies session cookies.
type Manager struct {
	secret []byte
	maxAge time.Duration
	secure bool
}

// New returns a Manager. `secret` must be at least 16 bytes. `secure` marks cookies
// Secure+SameSite=Lax (set true in prod; false for local HTTP tests).
func New(secret string, secure bool) (*Manager, error) {
	if len(secret) < 16 {
		return nil, fmt.Errorf("session secret must be at least 16 bytes")
	}
	return &Manager{
		secret: []byte(secret),
		maxAge: defaultMaxAge,
		secure: secure,
	}, nil
}

// Issue writes a new session cookie to w for the given Telegram user.
func (m *Manager) Issue(w http.ResponseWriter, telegramUserID int64) error {
	claims := Claims{
		TelegramUserID: telegramUserID,
		ExpiresAt:      time.Now().Add(m.maxAge).Unix(),
	}
	value, err := m.encode(claims)
	if err != nil {
		return err
	}
	http.SetCookie(w, &http.Cookie{
		Name:     CookieName,
		Value:    value,
		Path:     "/",
		Expires:  time.Unix(claims.ExpiresAt, 0),
		HttpOnly: true,
		Secure:   m.secure,
		SameSite: http.SameSiteLaxMode,
	})
	return nil
}

// Clear writes a past-dated cookie that instructs the browser to drop the session.
func (m *Manager) Clear(w http.ResponseWriter) {
	http.SetCookie(w, &http.Cookie{
		Name:     CookieName,
		Value:    "",
		Path:     "/",
		Expires:  time.Unix(1, 0),
		MaxAge:   -1,
		HttpOnly: true,
		Secure:   m.secure,
		SameSite: http.SameSiteLaxMode,
	})
}

// Verify reads and validates the session cookie. Returns the claims if valid.
func (m *Manager) Verify(r *http.Request) (Claims, error) {
	c, err := r.Cookie(CookieName)
	if err != nil {
		return Claims{}, ErrNoCookie
	}
	return m.decode(c.Value)
}

// ---- encoding internals ---------------------------------------------------

func (m *Manager) encode(c Claims) (string, error) {
	body, err := json.Marshal(c)
	if err != nil {
		return "", err
	}
	payload := base64.RawURLEncoding.EncodeToString(body)
	sig := m.sign(payload)
	return payload + "." + sig, nil
}

func (m *Manager) decode(value string) (Claims, error) {
	var payload, sig string
	for i := 0; i < len(value); i++ {
		if value[i] == '.' {
			payload, sig = value[:i], value[i+1:]
			break
		}
	}
	if payload == "" || sig == "" {
		return Claims{}, ErrMalformed
	}
	expected := m.sign(payload)
	if !hmac.Equal([]byte(sig), []byte(expected)) {
		return Claims{}, ErrBadSignature
	}
	body, err := base64.RawURLEncoding.DecodeString(payload)
	if err != nil {
		return Claims{}, ErrMalformed
	}
	var c Claims
	if err := json.Unmarshal(body, &c); err != nil {
		return Claims{}, ErrMalformed
	}
	if time.Now().Unix() > c.ExpiresAt {
		return Claims{}, ErrExpired
	}
	return c, nil
}

func (m *Manager) sign(payload string) string {
	h := hmac.New(sha256.New, m.secret)
	h.Write([]byte(payload))
	return base64.RawURLEncoding.EncodeToString(h.Sum(nil))
}
