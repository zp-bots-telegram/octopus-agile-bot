package tgauth

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"net/url"
	"sort"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// signFixture replicates Telegram's signing algorithm so tests can feed Verify a
// valid payload without making a real widget request.
func signFixture(t *testing.T, botToken string, params map[string]string) string {
	t.Helper()
	keys := make([]string, 0, len(params))
	for k := range params {
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

func TestVerify_HappyPath(t *testing.T) {
	botToken := "123:ABC"
	fields := map[string]string{
		"id":         "42",
		"first_name": "Zack",
		"auth_date":  strconv.FormatInt(time.Now().Unix(), 10),
	}
	fields["hash"] = signFixture(t, botToken, fields)

	d, err := Verify(botToken, fields)
	require.NoError(t, err)
	assert.Equal(t, int64(42), d.ID)
	assert.Equal(t, "Zack", d.FirstName)
}

func TestVerify_TamperedField(t *testing.T) {
	botToken := "123:ABC"
	fields := map[string]string{
		"id":        "42",
		"auth_date": strconv.FormatInt(time.Now().Unix(), 10),
	}
	fields["hash"] = signFixture(t, botToken, fields)
	fields["id"] = "43" // attacker changes id after signing

	_, err := Verify(botToken, fields)
	assert.ErrorIs(t, err, ErrSignatureMismatch)
}

func TestVerify_Stale(t *testing.T) {
	botToken := "123:ABC"
	old := time.Now().Add(-25 * time.Hour).Unix()
	fields := map[string]string{
		"id":        "42",
		"auth_date": strconv.FormatInt(old, 10),
	}
	fields["hash"] = signFixture(t, botToken, fields)

	_, err := Verify(botToken, fields)
	assert.ErrorIs(t, err, ErrStale)
}

func TestVerify_MissingHash(t *testing.T) {
	_, err := Verify("t", map[string]string{"id": "1", "auth_date": "1"})
	assert.ErrorIs(t, err, ErrSignatureMismatch)
}

// signInitData builds a valid initData string the same way Telegram would.
func signInitData(t *testing.T, botToken string, params map[string]string) string {
	t.Helper()
	keys := make([]string, 0, len(params))
	for k := range params {
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
	secretMac := hmac.New(sha256.New, []byte("WebAppData"))
	secretMac.Write([]byte(botToken))
	secretKey := secretMac.Sum(nil)
	mac := hmac.New(sha256.New, secretKey)
	mac.Write([]byte(sb.String()))
	hash := hex.EncodeToString(mac.Sum(nil))

	vals := url.Values{}
	for k, v := range params {
		vals.Set(k, v)
	}
	vals.Set("hash", hash)
	return vals.Encode()
}

func TestVerifyInitData_HappyPath(t *testing.T) {
	botToken := "123:ABC"
	user := `{"id":42,"first_name":"Zack","username":"zack"}`
	params := map[string]string{
		"auth_date": strconv.FormatInt(time.Now().Unix(), 10),
		"user":      user,
	}
	initData := signInitData(t, botToken, params)

	d, err := VerifyInitData(botToken, initData)
	require.NoError(t, err)
	assert.Equal(t, int64(42), d.ID)
	assert.Equal(t, "Zack", d.FirstName)
	assert.Equal(t, "zack", d.Username)
}

func TestVerifyInitData_Tampered(t *testing.T) {
	botToken := "123:ABC"
	params := map[string]string{
		"auth_date": strconv.FormatInt(time.Now().Unix(), 10),
		"user":      `{"id":42}`,
	}
	initData := signInitData(t, botToken, params)
	// Swap the user id AFTER signing.
	initData = strings.Replace(initData, "%22id%22%3A42", "%22id%22%3A43", 1)

	_, err := VerifyInitData(botToken, initData)
	assert.ErrorIs(t, err, ErrSignatureMismatch)
}

func TestVerifyInitData_Stale(t *testing.T) {
	botToken := "123:ABC"
	params := map[string]string{
		"auth_date": strconv.FormatInt(time.Now().Add(-25*time.Hour).Unix(), 10),
		"user":      `{"id":42}`,
	}
	_, err := VerifyInitData(botToken, signInitData(t, botToken, params))
	assert.ErrorIs(t, err, ErrStale)
}

func TestVerifyInitData_MissingHash(t *testing.T) {
	_, err := VerifyInitData("t", "auth_date=1&user=%7B%22id%22%3A1%7D")
	assert.ErrorIs(t, err, ErrSignatureMismatch)
}
