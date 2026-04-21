package config

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestLoad_Defaults(t *testing.T) {
	t.Setenv("TELEGRAM_BOT_TOKEN", "tg-token")
	t.Setenv("OCTOPUS_API_KEY", "sk_test")

	got, err := Load()
	require.NoError(t, err)
	assert.Equal(t, "C", got.DefaultRegion)
	assert.Equal(t, "/data/bot.db", got.DatabasePath)
	assert.Equal(t, "info", got.LogLevel)
	assert.Equal(t, "json", got.LogFormat)
	assert.Equal(t, "Europe/London", got.TZ)
	assert.NotNil(t, got.Location)
	assert.Empty(t, got.AllowedChatIDs)
}

func TestLoad_RegionNormalisedAndValidated(t *testing.T) {
	t.Setenv("TELEGRAM_BOT_TOKEN", "tg")
	t.Setenv("OCTOPUS_API_KEY", "ok")

	t.Setenv("DEFAULT_REGION", " h ")
	got, err := Load()
	require.NoError(t, err)
	assert.Equal(t, "H", got.DefaultRegion)

	t.Setenv("DEFAULT_REGION", "Z")
	_, err = Load()
	require.Error(t, err)

	t.Setenv("DEFAULT_REGION", "AB")
	_, err = Load()
	require.Error(t, err)
}

func TestLoad_RejectsBadLogFormat(t *testing.T) {
	t.Setenv("TELEGRAM_BOT_TOKEN", "tg")
	t.Setenv("OCTOPUS_API_KEY", "ok")
	t.Setenv("LOG_FORMAT", "yaml")

	_, err := Load()
	require.Error(t, err)
}

func TestLoad_RejectsBadTZ(t *testing.T) {
	t.Setenv("TELEGRAM_BOT_TOKEN", "tg")
	t.Setenv("OCTOPUS_API_KEY", "ok")
	t.Setenv("TZ", "Not/A/Zone")

	_, err := Load()
	require.Error(t, err)
}

func TestLoad_AllowedChatIDs(t *testing.T) {
	t.Setenv("TELEGRAM_BOT_TOKEN", "tg")
	t.Setenv("OCTOPUS_API_KEY", "ok")
	t.Setenv("ALLOWED_CHAT_IDS", "123,-456,789")

	got, err := Load()
	require.NoError(t, err)
	assert.Equal(t, []int64{123, -456, 789}, got.AllowedChatIDs)
	assert.True(t, got.IsChatAllowed(123))
	assert.True(t, got.IsChatAllowed(-456))
	assert.False(t, got.IsChatAllowed(999))
}

func TestIsChatAllowed_EmptyMeansPublic(t *testing.T) {
	c := Config{}
	assert.True(t, c.IsChatAllowed(1))
	assert.True(t, c.IsChatAllowed(-9999))
}

func TestLoad_RequiresSecrets(t *testing.T) {
	// Neither TELEGRAM_BOT_TOKEN nor OCTOPUS_API_KEY set.
	_, err := Load()
	require.Error(t, err)
}
