// Package tgauth verifies Telegram Login Widget payloads per
// https://core.telegram.org/widgets/login#checking-authorization.
package tgauth

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
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
