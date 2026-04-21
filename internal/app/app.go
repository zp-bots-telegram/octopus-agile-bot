// Package app owns the bot's startup/shutdown lifecycle. main.go is thin; everything
// that needs wiring lives here and is testable without running a real binary.
package app

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"strings"
	"time"

	"github.com/go-telegram/bot"
	"github.com/zp-bots-telegram/octopus-agile-bot/internal/agile"
	"github.com/zp-bots-telegram/octopus-agile-bot/internal/config"
	"github.com/zp-bots-telegram/octopus-agile-bot/internal/octopus"
	"github.com/zp-bots-telegram/octopus-agile-bot/internal/scheduler"
	"github.com/zp-bots-telegram/octopus-agile-bot/internal/service"
	"github.com/zp-bots-telegram/octopus-agile-bot/internal/storage"
	"github.com/zp-bots-telegram/octopus-agile-bot/internal/telegram"
)

type App struct {
	cfg   *config.Loaded
	log   *slog.Logger
	store *storage.Store
	bot   *bot.Bot
	sched *scheduler.Scheduler
}

// New builds every collaborator but does not start any goroutines.
func New(ctx context.Context, cfg *config.Loaded) (*App, error) {
	log := newLogger(cfg.LogFormat, cfg.LogLevel)

	store, err := storage.Open(ctx, cfg.DatabasePath)
	if err != nil {
		return nil, fmt.Errorf("open storage: %w", err)
	}

	octo := octopus.NewClient(cfg.OctopusAPIKey)

	b, err := bot.New(cfg.TelegramBotToken, bot.WithSkipGetMe())
	if err != nil {
		_ = store.Close()
		return nil, fmt.Errorf("bot.New: %w", err)
	}

	svc := service.New(service.Deps{
		Chats: store, Subs: store, Plans: store, Rates: store,
		Octopus:       octopusAdapter{c: octo},
		Notifier:      telegram.NewNotifier(b),
		Log:           log,
		DefaultTZ:     cfg.Location,
		DefaultRegion: cfg.DefaultRegion,
	})

	allow := cfg.Config.IsChatAllowed
	telegram.NewHandlers(svc, allow, log).Register(b)

	sch, err := scheduler.New(svc, cfg.Location, log)
	if err != nil {
		_ = store.Close()
		return nil, fmt.Errorf("scheduler.New: %w", err)
	}

	return &App{cfg: cfg, log: log, store: store, bot: b, sched: sch}, nil
}

// Run starts the scheduler and the bot's long-poll loop, and blocks until ctx is
// cancelled. Returns the first error encountered on shutdown.
func (a *App) Run(ctx context.Context) error {
	a.log.Info("starting",
		"tz", a.cfg.TZ,
		"default_region", a.cfg.DefaultRegion,
		"allowlist_size", len(a.cfg.AllowedChatIDs))

	if err := a.sched.Start(ctx); err != nil {
		return fmt.Errorf("scheduler.Start: %w", err)
	}

	// bot.Start blocks until ctx is done.
	done := make(chan struct{})
	go func() {
		a.bot.Start(ctx)
		close(done)
	}()

	<-ctx.Done()
	a.log.Info("shutdown signal received")

	// Bot.Start returns when ctx is cancelled — wait a moment for graceful exit.
	<-done
	if err := a.sched.Stop(); err != nil {
		a.log.Error("scheduler stop", "err", err)
	}
	if err := a.store.Close(); err != nil {
		a.log.Error("store close", "err", err)
	}
	return nil
}

// octopusAdapter narrows the octopus client to the service-facing interface.
type octopusAdapter struct{ c *octopus.Client }

func (a octopusAdapter) LatestAgileProduct(ctx context.Context) (service.ProductInfo, error) {
	p, err := a.c.LatestAgileProduct(ctx)
	if err != nil {
		return service.ProductInfo{}, err
	}
	return service.ProductInfo{Code: p.Code}, nil
}

func (a octopusAdapter) StandardUnitRates(
	ctx context.Context, productCode, tariffCode string, from, to time.Time,
) ([]agile.HalfHour, error) {
	return a.c.StandardUnitRates(ctx, productCode, tariffCode, from, to)
}

// newLogger builds a slog.Logger honouring LOG_FORMAT + LOG_LEVEL.
func newLogger(format, level string) *slog.Logger {
	var lvl slog.Level
	switch strings.ToLower(level) {
	case "debug":
		lvl = slog.LevelDebug
	case "warn", "warning":
		lvl = slog.LevelWarn
	case "error":
		lvl = slog.LevelError
	default:
		lvl = slog.LevelInfo
	}
	opts := &slog.HandlerOptions{Level: lvl}
	var h slog.Handler
	if strings.EqualFold(format, "text") {
		h = slog.NewTextHandler(os.Stderr, opts)
	} else {
		h = slog.NewJSONHandler(os.Stderr, opts)
	}
	return slog.New(h)
}
