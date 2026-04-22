package httpapi

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/zp-bots-telegram/octopus-agile-bot/internal/agile"
	"github.com/zp-bots-telegram/octopus-agile-bot/internal/service"
	"github.com/zp-bots-telegram/octopus-agile-bot/internal/session"
	"github.com/zp-bots-telegram/octopus-agile-bot/internal/storage"
)

type fakeOctopus struct{}

func (fakeOctopus) LatestAgileProduct(context.Context) (service.ProductInfo, error) {
	return service.ProductInfo{Code: "AGILE-X"}, nil
}
func (fakeOctopus) StandardUnitRates(context.Context, string, string, time.Time, time.Time) ([]agile.HalfHour, error) {
	return nil, nil
}
func (fakeOctopus) RegionForPostcode(_ context.Context, postcode string) (string, error) {
	if strings.HasPrefix(strings.ToUpper(postcode), "SW1A") {
		return "C", nil
	}
	return "A", nil
}
func (fakeOctopus) AccountWithKey(_ context.Context, apiKey, accountNumber string) (service.AccountInfo, error) {
	if apiKey == "bad" {
		return service.AccountInfo{}, fmt.Errorf("401 Unauthorized")
	}
	return service.AccountInfo{Number: accountNumber, CurrentTariff: "E-1R-AGILE-24-10-01-C"}, nil
}

type fakeNotifier struct{}

func (fakeNotifier) Notify(context.Context, int64, string) error { return nil }

func buildServer(t *testing.T) (*Server, *storage.Store, *session.Manager, string) {
	t.Helper()
	st, err := storage.Open(context.Background(), filepath.Join(t.TempDir(), "t.db"))
	require.NoError(t, err)
	t.Cleanup(func() { _ = st.Close() })

	tz, _ := time.LoadLocation("Europe/London")
	svc := service.New(service.Deps{
		Chats: st, Subs: st, Plans: st, Rates: st,
		Octopus: fakeOctopus{}, Notifier: fakeNotifier{},
		DefaultTZ: tz, DefaultRegion: "C",
	})

	secret := "supersecretvalue-16b+"
	mgr, err := session.New(secret, false)
	require.NoError(t, err)

	const botToken = "test-bot-token"
	srv := New(Deps{
		Service: svc, Sessions: mgr, BotToken: botToken,
	})
	return srv, st, mgr, botToken
}

// withSession returns a request with a valid session cookie for the given user ID.
func withSession(t *testing.T, method, path string, body []byte, mgr *session.Manager, userID int64) *http.Request {
	t.Helper()
	var r *http.Request
	if body != nil {
		r = httptest.NewRequest(method, path, bytes.NewReader(body))
		r.Header.Set("Content-Type", "application/json")
	} else {
		r = httptest.NewRequest(method, path, nil)
	}
	rec := httptest.NewRecorder()
	require.NoError(t, mgr.Issue(rec, userID))
	for _, c := range rec.Result().Cookies() {
		r.AddCookie(c)
	}
	return r
}

func TestHealth(t *testing.T) {
	srv, _, _, _ := buildServer(t)
	r := httptest.NewRequest(http.MethodGet, "/api/health", nil)
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, r)
	assert.Equal(t, http.StatusOK, w.Code)
}

func TestMe_Unauthenticated(t *testing.T) {
	srv, _, _, _ := buildServer(t)
	r := httptest.NewRequest(http.MethodGet, "/api/me", nil)
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, r)
	assert.Equal(t, http.StatusUnauthorized, w.Code)
}

func TestTelegramLogin_IssuesSession(t *testing.T) {
	srv, _, _, botToken := buildServer(t)

	params := map[string]string{
		"id":         "99",
		"first_name": "Zack",
		"auth_date":  strconv.FormatInt(time.Now().Unix(), 10),
	}
	params["hash"] = sign(botToken, params)

	body, _ := json.Marshal(params)
	r := httptest.NewRequest(http.MethodPost, "/api/auth/telegram/callback", bytes.NewReader(body))
	r.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, r)

	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	cookies := w.Result().Cookies()
	require.Len(t, cookies, 1)
	assert.Equal(t, session.CookieName, cookies[0].Name)
}

func TestTelegramLogin_BadSignature(t *testing.T) {
	srv, _, _, _ := buildServer(t)
	body, _ := json.Marshal(map[string]string{"id": "1", "hash": "zzz", "auth_date": "1"})
	r := httptest.NewRequest(http.MethodPost, "/api/auth/telegram/callback", bytes.NewReader(body))
	r.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, r)
	assert.Equal(t, http.StatusUnauthorized, w.Code)
}

func TestSetRegionByLetter(t *testing.T) {
	srv, _, mgr, _ := buildServer(t)
	body, _ := json.Marshal(map[string]string{"region": "h"})

	w := httptest.NewRecorder()
	srv.ServeHTTP(w, withSession(t, http.MethodPut, "/api/region", body, mgr, 77))
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())

	var resp map[string]string
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	assert.Equal(t, "H", resp["region"])
	assert.Equal(t, "Northern Scotland", resp["region_name"])
}

func TestSetRegionByPostcode(t *testing.T) {
	srv, _, mgr, _ := buildServer(t)
	body, _ := json.Marshal(map[string]string{"postcode": "SW1A 1AA"})

	w := httptest.NewRecorder()
	srv.ServeHTTP(w, withSession(t, http.MethodPut, "/api/region", body, mgr, 77))
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())

	var resp map[string]string
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	assert.Equal(t, "C", resp["region"])
}

func TestChargePlanCRUD(t *testing.T) {
	srv, _, mgr, _ := buildServer(t)

	// Set region first.
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, withSession(t, http.MethodPut, "/api/region",
		mustJSON(map[string]string{"region": "C"}), mgr, 7))
	require.Equal(t, http.StatusOK, w.Code)

	// Create plan.
	w = httptest.NewRecorder()
	srv.ServeHTTP(w, withSession(t, http.MethodPost, "/api/charge-plans",
		mustJSON(map[string]any{
			"duration_minutes":   240,
			"window_start_local": "22:00",
			"window_end_local":   "07:00",
		}), mgr, 7))
	require.Equal(t, http.StatusCreated, w.Code, w.Body.String())

	// List.
	w = httptest.NewRecorder()
	srv.ServeHTTP(w, withSession(t, http.MethodGet, "/api/charge-plans", nil, mgr, 7))
	require.Equal(t, http.StatusOK, w.Code)
	var plans []map[string]any
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &plans))
	require.Len(t, plans, 1)

	// Cancel.
	w = httptest.NewRecorder()
	srv.ServeHTTP(w, withSession(t, http.MethodDelete, "/api/charge-plans/1", nil, mgr, 7))
	assert.Equal(t, http.StatusNoContent, w.Code)
}

func TestCheapest_MissingDuration(t *testing.T) {
	srv, _, mgr, _ := buildServer(t)
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, withSession(t, http.MethodGet, "/api/cheapest", nil, mgr, 1))
	assert.Equal(t, http.StatusBadRequest, w.Code)
}

// ---- helpers -------------------------------------------------------------

func sign(botToken string, params map[string]string) string {
	keys := make([]string, 0, len(params))
	for k := range params {
		if k == "hash" {
			continue
		}
		keys = append(keys, k)
	}
	sort.Strings(keys)
	var sb strings.Builder
	for i, k := range keys {
		if i > 0 {
			sb.WriteByte('\n')
		}
		sb.WriteString(k)
		sb.WriteByte('=')
		sb.WriteString(params[k])
	}
	secret := sha256.Sum256([]byte(botToken))
	mac := hmac.New(sha256.New, secret[:])
	mac.Write([]byte(sb.String()))
	return hex.EncodeToString(mac.Sum(nil))
}

func mustJSON(v any) []byte {
	b, err := json.Marshal(v)
	if err != nil {
		panic(err)
	}
	return b
}
