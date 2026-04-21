package config

import (
	"fmt"
	"strings"
	"time"

	"github.com/caarlos0/env/v11"
)

type Config struct {
	TelegramBotToken string  `env:"TELEGRAM_BOT_TOKEN,required"`
	OctopusAPIKey    string  `env:"OCTOPUS_API_KEY,required"`
	DefaultRegion    string  `env:"DEFAULT_REGION" envDefault:"C"`
	DatabasePath     string  `env:"DATABASE_PATH" envDefault:"/data/bot.db"`
	LogLevel         string  `env:"LOG_LEVEL" envDefault:"info"`
	LogFormat        string  `env:"LOG_FORMAT" envDefault:"json"`
	TZ               string  `env:"TZ" envDefault:"Europe/London"`
	AllowedChatIDs   []int64 `env:"ALLOWED_CHAT_IDS" envSeparator:","`
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
	return &Loaded{Config: c, Location: loc}, nil
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
