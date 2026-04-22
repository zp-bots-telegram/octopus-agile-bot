// Package app owns the bot's startup/shutdown lifecycle. main.go is thin; everything
// that needs wiring lives here and is testable without running a real binary.
package app

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/go-telegram/bot"
	"github.com/zp-bots-telegram/octopus-agile-bot/internal/agile"
	"github.com/zp-bots-telegram/octopus-agile-bot/internal/config"
	"github.com/zp-bots-telegram/octopus-agile-bot/internal/cryptobox"
	"github.com/zp-bots-telegram/octopus-agile-bot/internal/httpapi"
	"github.com/zp-bots-telegram/octopus-agile-bot/internal/octopus"
	"github.com/zp-bots-telegram/octopus-agile-bot/internal/scheduler"
	"github.com/zp-bots-telegram/octopus-agile-bot/internal/service"
	"github.com/zp-bots-telegram/octopus-agile-bot/internal/session"
	"github.com/zp-bots-telegram/octopus-agile-bot/internal/storage"
	"github.com/zp-bots-telegram/octopus-agile-bot/internal/telegram"
)

type App struct {
	cfg      *config.Loaded
	log      *slog.Logger
	store    *storage.Store
	bot      *bot.Bot
	svc      *service.Service
	sched    *scheduler.Scheduler
	handlers *telegram.Handlers
	http     *http.Server
}

// New builds every collaborator but does not start any goroutines.
func New(ctx context.Context, cfg *config.Loaded) (*App, error) {
	log := newLogger(cfg.LogFormat, cfg.LogLevel)

	store, err := storage.Open(ctx, cfg.DatabasePath)
	if err != nil {
		return nil, fmt.Errorf("open storage: %w", err)
	}

	octo := octopus.NewClient(cfg.OctopusAPIKey)

	var cipher service.Cipher
	if cfg.EncryptionKey != "" {
		c, err := cryptobox.New([]byte(cfg.EncryptionKey))
		if err != nil {
			_ = store.Close()
			return nil, fmt.Errorf("cryptobox: %w", err)
		}
		cipher = c
	}

	b, err := bot.New(cfg.TelegramBotToken, bot.WithSkipGetMe())
	if err != nil {
		_ = store.Close()
		return nil, fmt.Errorf("bot.New: %w", err)
	}

	svc := service.New(service.Deps{
		Chats: store, Subs: store, Plans: store, Rates: store, PriceAlerts: store,
		OctopusLinks:  store,
		Cipher:        cipher,
		Octopus:       octopusAdapter{c: octo},
		Notifier:      telegram.NewNotifier(b),
		Log:           log,
		DefaultTZ:     cfg.Location,
		DefaultRegion: cfg.DefaultRegion,
	})

	allow := cfg.Config.IsChatAllowed
	handlers := telegram.NewHandlers(svc, allow, log)
	handlers.Register(b)

	sch, err := scheduler.New(svc, cfg.Location, log)
	if err != nil {
		_ = store.Close()
		return nil, fmt.Errorf("scheduler.New: %w", err)
	}

	// HTTP API for the web UI. Disabled unless SESSION_SECRET is set — signed cookies
	// are not negotiable, and we'd rather fail loudly than default to an insecure key.
	var httpSrv *http.Server
	if cfg.SessionSecret != "" {
		mgr, err := session.New(cfg.SessionSecret, strings.HasPrefix(cfg.WebBaseURL, "https://"))
		if err != nil {
			_ = store.Close()
			return nil, fmt.Errorf("session manager: %w", err)
		}
		// In dev, SvelteKit runs on :5173 and needs to call the Go API on :8080 with
		// credentials — permit that specific origin.
		devOrigin := os.Getenv("DEV_ORIGIN")
		api := httpapi.New(httpapi.Deps{
			Service: svc, Sessions: mgr, BotToken: cfg.TelegramBotToken, Log: log,
			DevOrigin: devOrigin,
		})
		httpSrv = &http.Server{
			Addr:              cfg.HTTPListenAddr,
			Handler:           api,
			ReadHeaderTimeout: 5 * time.Second,
		}
	} else {
		log.Info("HTTP API disabled — SESSION_SECRET not set")
	}

	return &App{cfg: cfg, log: log, store: store, bot: b, svc: svc, sched: sch, handlers: handlers, http: httpSrv}, nil
}

// Run starts the scheduler and the bot's long-poll loop, and blocks until ctx is
// cancelled. Returns the first error encountered on shutdown.
func (a *App) Run(ctx context.Context) error {
	a.log.Info("starting",
		"tz", a.cfg.TZ,
		"default_region", a.cfg.DefaultRegion,
		"allowlist_size", len(a.cfg.AllowedChatIDs))

	if err := a.handlers.PublishCommands(ctx, a.bot); err != nil {
		a.log.Warn("publish commands failed — menu may be stale", "err", err)
	} else {
		a.log.Info("telegram command menu published")
	}

	if err := a.sched.Start(ctx); err != nil {
		return fmt.Errorf("scheduler.Start: %w", err)
	}

	// Cold-start refresh. If the cache already has rates this is a harmless upsert;
	// if not, it's what makes /cheapest work on first use without waiting for the
	// scheduled 16:15 job. Runs in a goroutine so startup stays snappy.
	go func() {
		n, err := a.svc.RefreshRates(ctx)
		if err != nil {
			a.log.Warn("initial rate refresh failed", "err", err)
			return
		}
		a.log.Info("initial rate refresh complete", "rows", n)
	}()

	// HTTP server (optional).
	if a.http != nil {
		go func() {
			a.log.Info("http server listening", "addr", a.http.Addr)
			if err := a.http.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
				a.log.Error("http server", "err", err)
			}
		}()
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
	if a.http != nil {
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if err := a.http.Shutdown(shutdownCtx); err != nil {
			a.log.Error("http shutdown", "err", err)
		}
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

func (a octopusAdapter) RegionForPostcode(ctx context.Context, postcode string) (string, error) {
	return a.c.RegionForPostcode(ctx, postcode)
}

func (a octopusAdapter) AccountWithKey(ctx context.Context, apiKey, accountNumber string) (service.AccountInfo, error) {
	acc, err := a.c.WithAPIKey(apiKey).Account(ctx, accountNumber)
	if err != nil {
		return service.AccountInfo{}, err
	}
	info := service.AccountInfo{Number: acc.Number}
	for _, p := range acc.Properties {
		info.AddressLine1 = p.AddressLine1
		info.Postcode = p.Postcode
		for _, mp := range p.ElectricityMeterPoints {
			if mp.IsExport {
				continue
			}
			info.MPAN = mp.MPAN
			// Latest agreement = most recent valid_from.
			for _, ag := range mp.Agreements {
				if ag.ValidTo == "" || ag.ValidTo > time.Now().UTC().Format(time.RFC3339) {
					info.CurrentTariff = ag.TariffCode
				}
			}
			break
		}
		break
	}
	return info, nil
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
