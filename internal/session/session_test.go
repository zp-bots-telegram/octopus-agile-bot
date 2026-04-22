package session

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRoundTrip(t *testing.T) {
	m, err := New("supersecretvalue-16b+", false)
	require.NoError(t, err)

	w := httptest.NewRecorder()
	require.NoError(t, m.Issue(w, 42))

	cookies := w.Result().Cookies()
	require.Len(t, cookies, 1)
	require.Equal(t, CookieName, cookies[0].Name)

	r := httptest.NewRequest(http.MethodGet, "/", nil)
	r.AddCookie(cookies[0])
	c, err := m.Verify(r)
	require.NoError(t, err)
	assert.Equal(t, int64(42), c.TelegramUserID)
}

func TestVerify_TamperedPayload(t *testing.T) {
	m, _ := New("supersecretvalue-16b+", false)
	w := httptest.NewRecorder()
	require.NoError(t, m.Issue(w, 42))
	cookie := w.Result().Cookies()[0]

	// Flip one byte in the payload half.
	dot := strings.IndexByte(cookie.Value, '.')
	require.Greater(t, dot, 0)
	tampered := cookie.Value[:dot-1] + "A" + cookie.Value[dot:]
	cookie.Value = tampered

	r := httptest.NewRequest(http.MethodGet, "/", nil)
	r.AddCookie(cookie)
	_, err := m.Verify(r)
	assert.ErrorIs(t, err, ErrBadSignature)
}

func TestVerify_NoCookie(t *testing.T) {
	m, _ := New("supersecretvalue-16b+", false)
	r := httptest.NewRequest(http.MethodGet, "/", nil)
	_, err := m.Verify(r)
	assert.ErrorIs(t, err, ErrNoCookie)
}

func TestVerify_Malformed(t *testing.T) {
	m, _ := New("supersecretvalue-16b+", false)
	r := httptest.NewRequest(http.MethodGet, "/", nil)
	r.AddCookie(&http.Cookie{Name: CookieName, Value: "not-a-session"})
	_, err := m.Verify(r)
	assert.ErrorIs(t, err, ErrMalformed)
}

func TestNew_RejectsShortSecret(t *testing.T) {
	_, err := New("short", false)
	require.Error(t, err)
}

func TestClear_SetsExpiredCookie(t *testing.T) {
	m, _ := New("supersecretvalue-16b+", false)
	w := httptest.NewRecorder()
	m.Clear(w)
	cookie := w.Result().Cookies()[0]
	assert.Equal(t, CookieName, cookie.Name)
	assert.Equal(t, "", cookie.Value)
	assert.Equal(t, -1, cookie.MaxAge)
}
