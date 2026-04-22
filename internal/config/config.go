package config

import (
	"fmt"
	"strings"
	"time"

	"github.com/caarlos0/env/v11"
)

type Config struct {
	// Core / telegram.
	TelegramBotToken string  `env:"TELEGRAM_BOT_TOKEN,required"`
	OctopusAPIKey    string  `env:"OCTOPUS_API_KEY,required"`
	DefaultRegion    string  `env:"DEFAULT_REGION" envDefault:"C"`
	DatabasePath     string  `env:"DATABASE_PATH" envDefault:"/data/bot.db"`
	LogLevel         string  `env:"LOG_LEVEL" envDefault:"info"`
	LogFormat        string  `env:"LOG_FORMAT" envDefault:"json"`
	TZ               string  `env:"TZ" envDefault:"Europe/London"`
	AllowedChatIDs   []int64 `env:"ALLOWED_CHAT_IDS" envSeparator:","`

	// Web UI. WebBaseURL is the public URL of the web app (used for OAuth callbacks
	// and Telegram Login Widget registration). HTTPListenAddr is where the Go
	// process listens. SessionSecret signs web session cookies — must be ≥ 32 bytes
	// of high-entropy bytes in prod.
	HTTPListenAddr string `env:"HTTP_LISTEN_ADDR" envDefault:":8080"`
	WebBaseURL     string `env:"WEB_BASE_URL" envDefault:"http://localhost:8080"`
	SessionSecret  string `env:"SESSION_SECRET"`
	EncryptionKey  string `env:"ENCRYPTION_KEY"`

	// Octopus OAuth (phase 2). All optional for now; the OAuth handler is disabled
	// unless both ClientID and TokenURL are set.
	OctopusOAuthClientID     string `env:"OCTOPUS_OAUTH_CLIENT_ID"`
	OctopusOAuthClientSecret string `env:"OCTOPUS_OAUTH_CLIENT_SECRET"`
	OctopusOAuthAuthorizeURL string `env:"OCTOPUS_OAUTH_AUTHORIZE_URL"`
	OctopusOAuthTokenURL     string `env:"OCTOPUS_OAUTH_TOKEN_URL"`
	OctopusOAuthScopes       string `env:"OCTOPUS_OAUTH_SCOPES"`
}

type Loaded struct {
	Config
	Location *time.Location
}

func Load() (*Loaded, error) {
	var c Config
	if err := env.Parse(&c); err != nil {
		return nil, fmt.Errorf("config: %w", err)
	}
	c.DefaultRegion = strings.ToUpper(strings.TrimSpace(c.DefaultRegion))
	if !validRegion(c.DefaultRegion) {
		return nil, fmt.Errorf("DEFAULT_REGION must be a single letter A-P, got %q", c.DefaultRegion)
	}
	loc, err := time.LoadLocation(c.TZ)
	if err != nil {
		return nil, fmt.Errorf("TZ %q invalid: %w", c.TZ, err)
	}
	switch c.LogFormat {
	case "json", "text":
	default:
		return nil, fmt.Errorf("LOG_FORMAT must be json|text, got %q", c.LogFormat)
	}
	if c.SessionSecret != "" && len(c.SessionSecret) < 16 {
		return nil, fmt.Errorf("SESSION_SECRET must be at least 16 bytes")
	}
	if c.EncryptionKey != "" && len(c.EncryptionKey) != 32 {
		return nil, fmt.Errorf("ENCRYPTION_KEY must be exactly 32 bytes (AES-256)")
	}
	return &Loaded{Config: c, Location: loc}, nil
}

// OctopusOAuthEnabled reports whether enough config is set to run the OAuth flow.
func (c Config) OctopusOAuthEnabled() bool {
	return c.OctopusOAuthClientID != "" && c.OctopusOAuthTokenURL != "" && c.OctopusOAuthAuthorizeURL != ""
}

// IsChatAllowed reports whether the given chat ID may interact with the bot.
// An empty allowlist means "everyone".
func (c Config) IsChatAllowed(chatID int64) bool {
	if len(c.AllowedChatIDs) == 0 {
		return true
	}
	for _, id := range c.AllowedChatIDs {
		if id == chatID {
			return true
		}
	}
	return false
}

func validRegion(r string) bool {
	return len(r) == 1 && r[0] >= 'A' && r[0] <= 'P'
}
