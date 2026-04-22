package tgauth

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
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
