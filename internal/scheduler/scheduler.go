// Package scheduler wires the time-driven jobs (daily Agile refresh, charge-plan
// dispatch, per-user subscription notifications) to the service layer. The scheduler
// holds no business logic — it only decides *when*.
package scheduler

import (
	"context"
	"fmt"
	"log/slog"
	"math"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/go-co-op/gocron/v2"
	"github.com/google/uuid"
	"github.com/zp-bots-telegram/octopus-agile-bot/internal/service"
	"github.com/zp-bots-telegram/octopus-agile-bot/internal/storage"
)

// Scheduler owns a gocron.Scheduler and a mapping of subscription chat_id → job id so
// subscriptions can be added/removed at runtime.
type Scheduler struct {
	svc *service.Service
	log *slog.Logger

	refreshTZ *time.Location
	sched     gocron.Scheduler

	mu      sync.Mutex
	subJobs map[int64]uuid.UUID
}

func New(svc *service.Service, refreshTZ *time.Location, log *slog.Logger) (*Scheduler, error) {
	if log == nil {
		log = slog.Default()
	}
	if refreshTZ == nil {
		refreshTZ, _ = time.LoadLocation("Europe/London")
	}
	sch, err := gocron.NewScheduler(gocron.WithLocation(refreshTZ))
	if err != nil {
		return nil, err
	}
	return &Scheduler{
		svc: svc, log: log, refreshTZ: refreshTZ,
		sched:   sch,
		subJobs: map[int64]uuid.UUID{},
	}, nil
}

// Start registers the fixed daily jobs, registers every existing subscription, and
// kicks off the scheduler goroutine. It does NOT block.
func (s *Scheduler) Start(ctx context.Context) error {
	// Daily Agile rate refresh at 16:15 local (post-publication), then dispatch.
	_, err := s.sched.NewJob(
		gocron.DailyJob(1, gocron.NewAtTimes(gocron.NewAtTime(16, 15, 0))),
		gocron.NewTask(s.runRefreshAndDispatch, ctx),
		gocron.WithName("daily-agile-refresh"),
	)
	if err != nil {
		return fmt.Errorf("register daily refresh: %w", err)
	}

	// Price-alert dispatcher: tick every minute so the lead-time window (9–12 min
	// before a run starts) is reliably hit.
	_, err = s.sched.NewJob(
		gocron.DurationJob(time.Minute),
		gocron.NewTask(func() {
			if _, err := s.svc.DispatchPriceAlerts(ctx); err != nil {
				s.log.Error("price alert dispatch", "err", err)
			}
		}),
		gocron.WithName("price-alerts"),
	)
	if err != nil {
		return fmt.Errorf("register price alerts: %w", err)
	}

	subs, err := s.subscriptions(ctx)
	if err != nil {
		return fmt.Errorf("load subs: %w", err)
	}
	for _, sub := range subs {
		if err := s.AddSubscriptionJob(ctx, sub); err != nil {
			s.log.Error("register sub", "chat_id", sub.ChatID, "err", err)
		}
	}

	s.sched.Start()
	s.log.Info("scheduler started",
		"location", s.refreshTZ.String(),
		"subscriptions", len(subs))
	return nil
}

// Stop shuts down the scheduler and waits for running jobs.
func (s *Scheduler) Stop() error { return s.sched.Shutdown() }

// ---- job implementations --------------------------------------------------

func (s *Scheduler) runRefreshAndDispatch(ctx context.Context) {
	backoff := time.Minute
	deadline := time.Now().Add(1 * time.Hour)
	for attempt := 1; ; attempt++ {
		n, err := s.svc.RefreshRates(ctx)
		if err == nil {
			s.log.Info("rates refreshed", "rows", n, "attempt", attempt)
			break
		}
		s.log.Warn("rate refresh failed", "attempt", attempt, "err", err)
		if time.Now().After(deadline) {
			s.log.Error("rate refresh exhausted retries", "err", err)
			return
		}
		select {
		case <-ctx.Done():
			return
		case <-time.After(backoff):
		}
		backoff = time.Duration(math.Min(float64(backoff)*2, float64(15*time.Minute)))
	}

	if _, err := s.svc.DispatchTodaysChargePlans(ctx); err != nil {
		s.log.Error("dispatch charge plans", "err", err)
	}
}

// ---- subscription management ---------------------------------------------

func (s *Scheduler) subscriptions(ctx context.Context) ([]storage.Subscription, error) {
	// The scheduler only needs the service layer; expose via a small helper since
	// ListEnabledSubscriptions is a repo method, not a service method.
	return s.svc.EnabledSubscriptions(ctx)
}

// AddSubscriptionJob registers a daily notification for one chat. If a job already
// exists for the chat it is replaced — handy when the user changes their notify time.
func (s *Scheduler) AddSubscriptionJob(ctx context.Context, sub storage.Subscription) error {
	hh, mm, err := parseHHMM(sub.NotifyAtLocal)
	if err != nil {
		return err
	}

	s.mu.Lock()
	if existing, ok := s.subJobs[sub.ChatID]; ok {
		if err := s.sched.RemoveJob(existing); err != nil {
			s.log.Warn("remove prev sub job", "chat_id", sub.ChatID, "err", err)
		}
		delete(s.subJobs, sub.ChatID)
	}
	s.mu.Unlock()

	chatID := sub.ChatID
	job, err := s.sched.NewJob(
		gocron.DailyJob(1, gocron.NewAtTimes(gocron.NewAtTime(uint(hh), uint(mm), 0))),
		gocron.NewTask(func() {
			if err := s.svc.DispatchSubscription(ctx, chatID); err != nil {
				s.log.Error("sub dispatch", "chat_id", chatID, "err", err)
			}
		}),
		gocron.WithName("sub-"+strconv.FormatInt(chatID, 10)),
	)
	if err != nil {
		return fmt.Errorf("register sub job: %w", err)
	}

	s.mu.Lock()
	s.subJobs[chatID] = job.ID()
	s.mu.Unlock()
	return nil
}

func (s *Scheduler) RemoveSubscriptionJob(chatID int64) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if id, ok := s.subJobs[chatID]; ok {
		_ = s.sched.RemoveJob(id)
		delete(s.subJobs, chatID)
	}
}

func parseHHMM(s string) (int, int, error) {
	t, err := time.Parse("15:04", strings.TrimSpace(s))
	if err != nil {
		return 0, 0, err
	}
	return t.Hour(), t.Minute(), nil
}
