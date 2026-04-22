// Package tgauth verifies Telegram authentication payloads:
//   - Login Widget, per https://core.telegram.org/widgets/login#checking-authorization.
//   - Mini App initData, per
//     https://core.telegram.org/bots/webapps#validating-data-received-via-the-mini-app.
//
// The two flows share the same overall shape (k/v pairs + a hash) but use different
// secret-key derivations — see Verify vs VerifyInitData.
package tgauth

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"sort"
	"strconv"
	"strings"
	"time"
)

// LoginData is the subset of fields we care about from the widget. All values are
// the raw strings Telegram put in the URL fragment.
type LoginData struct {
	ID        int64
	FirstName string
	LastName  string
	Username  string
	PhotoURL  string
	AuthDate  int64
}

var (
	ErrSignatureMismatch = errors.New("telegram login: signature mismatch")
	ErrStale             = errors.New("telegram login: auth_date too old")
)

// MaxAge is how long a login payload remains valid. Telegram recommends 24h.
const MaxAge = 24 * time.Hour

// Verify checks the payload's HMAC against the bot token and that auth_date is
// within MaxAge. `params` is the raw key/value map from the callback (including a
// `hash` field).
func Verify(botToken string, params map[string]string) (LoginData, error) {
	hash, ok := params["hash"]
	if !ok || hash == "" {
		return LoginData{}, ErrSignatureMismatch
	}

	// Build the data-check string: sorted k=v joined by '\n', excluding `hash`.
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

	secretKey := sha256.Sum256([]byte(botToken))
	mac := hmac.New(sha256.New, secretKey[:])
	mac.Write([]byte(sb.String()))
	got := hex.EncodeToString(mac.Sum(nil))
	if !hmac.Equal([]byte(got), []byte(hash)) {
		return LoginData{}, ErrSignatureMismatch
	}

	var d LoginData
	if v, err := strconv.ParseInt(params["id"], 10, 64); err == nil {
		d.ID = v
	} else {
		return LoginData{}, fmt.Errorf("telegram login: bad id: %w", err)
	}
	if v, err := strconv.ParseInt(params["auth_date"], 10, 64); err == nil {
		d.AuthDate = v
	} else {
		return LoginData{}, fmt.Errorf("telegram login: bad auth_date: %w", err)
	}
	d.FirstName = params["first_name"]
	d.LastName = params["last_name"]
	d.Username = params["username"]
	d.PhotoURL = params["photo_url"]

	age := time.Since(time.Unix(d.AuthDate, 0))
	if age > MaxAge {
		return LoginData{}, ErrStale
	}
	return d, nil
}

// VerifyInitData validates the `initData` string produced by the Mini App runtime
// (window.Telegram.WebApp.initData) and returns the contained user data on success.
//
// Unlike the Login Widget, the secret key is HMAC-SHA256(key="WebAppData",
// message=bot_token); otherwise the data-check-string + hash comparison is the same.
func VerifyInitData(botToken, initData string) (LoginData, error) {
	// initData is a urlencoded query string.
	values, err := url.ParseQuery(initData)
	if err != nil {
		return LoginData{}, fmt.Errorf("telegram initdata: parse: %w", err)
	}
	hash := values.Get("hash")
	if hash == "" {
		return LoginData{}, ErrSignatureMismatch
	}

	keys := make([]string, 0, len(values))
	for k := range values {
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
		sb.WriteString(values.Get(k))
	}

	// secret_key = HMAC_SHA256("WebAppData", bot_token)
	secretMac := hmac.New(sha256.New, []byte("WebAppData"))
	secretMac.Write([]byte(botToken))
	secretKey := secretMac.Sum(nil)

	mac := hmac.New(sha256.New, secretKey)
	mac.Write([]byte(sb.String()))
	got := hex.EncodeToString(mac.Sum(nil))
	if !hmac.Equal([]byte(got), []byte(hash)) {
		return LoginData{}, ErrSignatureMismatch
	}

	var d LoginData
	if v, err := strconv.ParseInt(values.Get("auth_date"), 10, 64); err == nil {
		d.AuthDate = v
	} else {
		return LoginData{}, fmt.Errorf("telegram initdata: bad auth_date: %w", err)
	}
	if time.Since(time.Unix(d.AuthDate, 0)) > MaxAge {
		return LoginData{}, ErrStale
	}

	// `user` is a JSON-encoded object. Field names use snake_case.
	if u := values.Get("user"); u != "" {
		var user struct {
			ID        int64  `json:"id"`
			FirstName string `json:"first_name"`
			LastName  string `json:"last_name"`
			Username  string `json:"username"`
			PhotoURL  string `json:"photo_url"`
		}
		if err := json.Unmarshal([]byte(u), &user); err != nil {
			return LoginData{}, fmt.Errorf("telegram initdata: parse user: %w", err)
		}
		d.ID = user.ID
		d.FirstName = user.FirstName
		d.LastName = user.LastName
		d.Username = user.Username
		d.PhotoURL = user.PhotoURL
	}
	if d.ID == 0 {
		return LoginData{}, fmt.Errorf("telegram initdata: missing user.id")
	}
	return d, nil
}
