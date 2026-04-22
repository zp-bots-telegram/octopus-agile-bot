// Package service holds the transport-agnostic use-cases of the bot. Everything that
// needs to do side effects (HTTP, DB, outgoing Telegram messages, scheduling) goes
// through interfaces defined in this package, so both the Telegram transport and a
// future web UI can call the same code.
package service

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/zp-bots-telegram/octopus-agile-bot/internal/agile"
	"github.com/zp-bots-telegram/octopus-agile-bot/internal/storage"
)

// ---- ports (consumer-owned interfaces) -----------------------------------

type ChatRepo interface {
	UpsertChatRegion(ctx context.Context, chatID int64, region string) error
	SetChatTimezone(ctx context.Context, chatID int64, tz string) error
	GetChat(ctx context.Context, chatID int64) (storage.Chat, bool, error)
	DistinctRegions(ctx context.Context) ([]string, error)
}

type SubscriptionRepo interface {
	SetSubscription(ctx context.Context, sub storage.Subscription) error
	DeleteSubscription(ctx context.Context, chatID int64) error
	GetSubscription(ctx context.Context, chatID int64) (storage.Subscription, bool, error)
	ListEnabledSubscriptions(ctx context.Context) ([]storage.Subscription, error)
}

type ChargePlanRepo interface {
	CreateChargePlan(ctx context.Context, p storage.ChargePlan) (int64, error)
	ListChargePlans(ctx context.Context, chatID int64) ([]storage.ChargePlan, error)
	ListEnabledChargePlans(ctx context.Context) ([]storage.ChargePlan, error)
	CancelChargePlan(ctx context.Context, chatID, id int64) (bool, error)
	MarkChargePlanDispatched(ctx context.Context, chatID, planID int64, targetDate time.Time) (bool, error)
}

type RateRepo interface {
	UpsertRates(ctx context.Context, region, tariffCode string, rates []agile.HalfHour) error
	Rates(ctx context.Context, region string, from, to time.Time) ([]agile.HalfHour, error)
}

// OctopusClient is the subset of the external Octopus API we depend on. Kept as an
// interface so tests can stub it without standing up an HTTP server.
type OctopusClient interface {
	LatestAgileProduct(ctx context.Context) (ProductInfo, error)
	StandardUnitRates(ctx context.Context, productCode, tariffCode string, from, to time.Time) ([]agile.HalfHour, error)
	RegionForPostcode(ctx context.Context, postcode string) (string, error)
}

// ProductInfo is the service-layer view of an Octopus product — only what we need.
type ProductInfo struct {
	Code string
}

// Notifier sends a plain-text message to a chat. Implemented by the Telegram transport
// in prod, by a fake in tests, and potentially by push-to-web-UI later.
type Notifier interface {
	Notify(ctx context.Context, chatID int64, text string) error
}

// Clock is abstracted so tests can drive the scheduler deterministically.
type Clock interface {
	Now() time.Time
}

type realClock struct{}

func (realClock) Now() time.Time { return time.Now() }

// ---- service --------------------------------------------------------------

type Service struct {
	chats     ChatRepo
	subs      SubscriptionRepo
	plans     ChargePlanRepo
	rates     RateRepo
	octopus   OctopusClient
	notifier  Notifier
	log       *slog.Logger
	clock     Clock
	defaultTZ *time.Location
	defaultRg string
}

type Deps struct {
	Chats    ChatRepo
	Subs     SubscriptionRepo
	Plans    ChargePlanRepo
	Rates    RateRepo
	Octopus  OctopusClient
	Notifier Notifier
	Log      *slog.Logger
	Clock    Clock
	// DefaultTZ is used for chats that haven't set their own.
	DefaultTZ *time.Location
	// DefaultRegion is used before /region is set; must be a single letter A-P.
	DefaultRegion string
}

func New(d Deps) *Service {
	if d.Clock == nil {
		d.Clock = realClock{}
	}
	if d.Log == nil {
		d.Log = slog.Default()
	}
	if d.DefaultTZ == nil {
		d.DefaultTZ, _ = time.LoadLocation("Europe/London")
	}
	return &Service{
		chats:     d.Chats,
		subs:      d.Subs,
		plans:     d.Plans,
		rates:     d.Rates,
		octopus:   d.Octopus,
		notifier:  d.Notifier,
		log:       d.Log,
		clock:     d.Clock,
		defaultTZ: d.DefaultTZ,
		defaultRg: strings.ToUpper(d.DefaultRegion),
	}
}

// ---- user-facing use-cases ------------------------------------------------

var (
	ErrInvalidRegion = errors.New("region must be a single letter A-P")
	ErrNoChat        = errors.New("no chat record; set a region first with /region")
	ErrBadTime       = errors.New("time must be HH:MM (24h)")
)

func (s *Service) SetRegion(ctx context.Context, chatID int64, region string) error {
	r := strings.ToUpper(strings.TrimSpace(region))
	if len(r) != 1 || r[0] < 'A' || r[0] > 'P' {
		return ErrInvalidRegion
	}
	return s.chats.UpsertChatRegion(ctx, chatID, r)
}

// SetRegionByPostcode resolves a UK postcode to a DNO region letter via the Octopus
// industry GSP endpoint and persists it for the chat. Returns the resolved letter.
func (s *Service) SetRegionByPostcode(ctx context.Context, chatID int64, postcode string) (string, error) {
	r, err := s.octopus.RegionForPostcode(ctx, postcode)
	if err != nil {
		return "", err
	}
	if err := s.SetRegion(ctx, chatID, r); err != nil {
		return "", err
	}
	return r, nil
}

// resolveChat returns the chat's stored region/timezone, or synthesises a default chat
// if none exists yet (so /cheapest works on first use with the env default region).
func (s *Service) resolveChat(ctx context.Context, chatID int64) (storage.Chat, error) {
	c, ok, err := s.chats.GetChat(ctx, chatID)
	if err != nil {
		return storage.Chat{}, err
	}
	if ok {
		return c, nil
	}
	if s.defaultRg == "" {
		return storage.Chat{}, ErrNoChat
	}
	return storage.Chat{ChatID: chatID, Region: s.defaultRg, Timezone: s.defaultTZ.String()}, nil
}

// CheapestWindow finds the cheapest `duration` window within the horizon of currently
// published rates for the given chat.
func (s *Service) CheapestWindow(ctx context.Context, chatID int64, duration time.Duration) (agile.Window, error) {
	chat, err := s.resolveChat(ctx, chatID)
	if err != nil {
		return agile.Window{}, err
	}
	now := s.clock.Now().UTC()
	horizon := now.Add(48 * time.Hour)
	rates, err := s.rates.Rates(ctx, chat.Region, now, horizon)
	if err != nil {
		return agile.Window{}, err
	}
	return agile.CheapestWindow(rates, duration, now, horizon)
}

// NextBelowThreshold returns the next half-hour for this chat's region whose inc-VAT
// price is strictly less than `threshold`.
func (s *Service) NextBelowThreshold(ctx context.Context, chatID int64, threshold float64) (agile.HalfHour, error) {
	chat, err := s.resolveChat(ctx, chatID)
	if err != nil {
		return agile.HalfHour{}, err
	}
	now := s.clock.Now().UTC()
	rates, err := s.rates.Rates(ctx, chat.Region, now, now.Add(48*time.Hour))
	if err != nil {
		return agile.HalfHour{}, err
	}
	return agile.NextBelowThreshold(rates, threshold, now)
}

// SetSubscription creates or updates the chat's daily "cheapest window over next 24h"
// notification.
func (s *Service) SetSubscription(ctx context.Context, chatID int64, duration time.Duration, notifyAtLocal string) error {
	if _, err := parseHHMM(notifyAtLocal); err != nil {
		return err
	}
	return s.subs.SetSubscription(ctx, storage.Subscription{
		ChatID: chatID, Duration: duration,
		NotifyAtLocal: notifyAtLocal, Enabled: true,
	})
}

func (s *Service) Unsubscribe(ctx context.Context, chatID int64) error {
	return s.subs.DeleteSubscription(ctx, chatID)
}

// EnabledSubscriptions exposes the scheduler-facing subscription list. Kept on the
// service so the scheduler need not know about the storage package directly.
func (s *Service) EnabledSubscriptions(ctx context.Context) ([]storage.Subscription, error) {
	return s.subs.ListEnabledSubscriptions(ctx)
}

// CreateChargePlan registers a recurring daily charging plan for the chat.
func (s *Service) CreateChargePlan(ctx context.Context, chatID int64, duration time.Duration, startLocal, endLocal string) (storage.ChargePlan, error) {
	if _, err := parseHHMM(startLocal); err != nil {
		return storage.ChargePlan{}, err
	}
	if _, err := parseHHMM(endLocal); err != nil {
		return storage.ChargePlan{}, err
	}
	p := storage.ChargePlan{
		ChatID: chatID, Duration: duration,
		WindowStartLocal: startLocal, WindowEndLocal: endLocal, Enabled: true,
	}
	id, err := s.plans.CreateChargePlan(ctx, p)
	if err != nil {
		return storage.ChargePlan{}, err
	}
	p.ID = id
	return p, nil
}

func (s *Service) ListChargePlans(ctx context.Context, chatID int64) ([]storage.ChargePlan, error) {
	return s.plans.ListChargePlans(ctx, chatID)
}

func (s *Service) CancelChargePlan(ctx context.Context, chatID, id int64) (bool, error) {
	return s.plans.CancelChargePlan(ctx, chatID, id)
}

// Status is the aggregated view displayed by /status.
type Status struct {
	Region       string
	Timezone     string
	Subscription *storage.Subscription
	ChargePlans  []storage.ChargePlan
}

func (s *Service) Status(ctx context.Context, chatID int64) (Status, error) {
	chat, err := s.resolveChat(ctx, chatID)
	if err != nil {
		return Status{}, err
	}
	sub, has, err := s.subs.GetSubscription(ctx, chatID)
	if err != nil {
		return Status{}, err
	}
	plans, err := s.plans.ListChargePlans(ctx, chatID)
	if err != nil {
		return Status{}, err
	}
	st := Status{Region: chat.Region, Timezone: chat.Timezone, ChargePlans: plans}
	if has {
		st.Subscription = &sub
	}
	return st, nil
}

// ---- scheduled use-cases --------------------------------------------------

// RefreshRates pulls the latest Agile rates for every region in use and persists them.
// Safe to call repeatedly; UpsertRates handles dedup. Returns the number of rows upserted.
func (s *Service) RefreshRates(ctx context.Context) (int, error) {
	regions, err := s.chats.DistinctRegions(ctx)
	if err != nil {
		return 0, err
	}
	if len(regions) == 0 && s.defaultRg != "" {
		regions = []string{s.defaultRg}
	}
	if len(regions) == 0 {
		return 0, nil
	}
	prod, err := s.octopus.LatestAgileProduct(ctx)
	if err != nil {
		return 0, fmt.Errorf("latest agile product: %w", err)
	}
	now := s.clock.Now().UTC()
	from := now.Truncate(30 * time.Minute)
	to := from.Add(48 * time.Hour)

	total := 0
	for _, r := range regions {
		tc := agile.TariffCode(prod.Code, r)
		rates, err := s.octopus.StandardUnitRates(ctx, prod.Code, tc, from, to)
		if err != nil {
			return total, fmt.Errorf("rates for region %s: %w", r, err)
		}
		if err := s.rates.UpsertRates(ctx, r, tc, rates); err != nil {
			return total, fmt.Errorf("persist region %s: %w", r, err)
		}
		total += len(rates)
		s.log.Info("rates refreshed", "region", r, "count", len(rates))
	}
	return total, nil
}

// Dispatch is one output of DispatchTodaysChargePlans.
type Dispatch struct {
	Plan   storage.ChargePlan
	Window agile.Window
}

// DispatchTodaysChargePlans evaluates every active charge plan against tonight's rates
// and notifies the owning chat. Each (chat, plan, date) triple is dispatched at most
// once per day.
func (s *Service) DispatchTodaysChargePlans(ctx context.Context) ([]Dispatch, error) {
	plans, err := s.plans.ListEnabledChargePlans(ctx)
	if err != nil {
		return nil, err
	}
	now := s.clock.Now()
	var done []Dispatch

	for _, p := range plans {
		chat, _, err := s.chats.GetChat(ctx, p.ChatID)
		if err != nil {
			s.log.Error("get chat for plan", "chat_id", p.ChatID, "err", err)
			continue
		}
		tz, err := time.LoadLocation(chat.Timezone)
		if err != nil {
			tz = s.defaultTZ
		}

		start, _ := parseHHMM(p.WindowStartLocal)
		end, _ := parseHHMM(p.WindowEndLocal)

		// Anchor the plan to "today" in the chat's timezone — the window may run into
		// tomorrow morning if it crosses midnight.
		anchor := now.In(tz)
		y, m, d := anchor.Date()
		from := time.Date(y, m, d, start.Hour(), start.Minute(), 0, 0, tz)
		to := time.Date(y, m, d, end.Hour(), end.Minute(), 0, 0, tz)
		if !to.After(from) {
			to = to.AddDate(0, 0, 1)
		}

		rates, err := s.rates.Rates(ctx, chat.Region, from.UTC(), to.UTC())
		if err != nil {
			s.log.Error("rates for plan", "chat_id", p.ChatID, "plan_id", p.ID, "err", err)
			continue
		}
		w, err := agile.CheapestWindow(rates, p.Duration, from.UTC(), to.UTC())
		if err != nil {
			s.log.Warn("cheapest window for plan", "chat_id", p.ChatID, "plan_id", p.ID, "err", err)
			continue
		}

		fresh, err := s.plans.MarkChargePlanDispatched(ctx, p.ChatID, p.ID, time.Date(y, m, d, 0, 0, 0, 0, tz))
		if err != nil {
			s.log.Error("mark dispatched", "err", err)
			continue
		}
		if !fresh {
			continue // already dispatched today
		}

		msg := FormatChargeDispatch(p, w, tz)
		if err := s.notifier.Notify(ctx, p.ChatID, msg); err != nil {
			s.log.Error("notify charge plan", "chat_id", p.ChatID, "err", err)
			continue
		}
		done = append(done, Dispatch{Plan: p, Window: w})
	}
	return done, nil
}

// DispatchSubscription sends a single /subscribe notification.
func (s *Service) DispatchSubscription(ctx context.Context, chatID int64) error {
	sub, ok, err := s.subs.GetSubscription(ctx, chatID)
	if err != nil {
		return err
	}
	if !ok || !sub.Enabled {
		return nil
	}
	w, err := s.CheapestWindow(ctx, chatID, sub.Duration)
	if err != nil {
		return err
	}
	chat, _, err := s.chats.GetChat(ctx, chatID)
	if err != nil {
		return err
	}
	tz, err := time.LoadLocation(chat.Timezone)
	if err != nil {
		tz = s.defaultTZ
	}
	return s.notifier.Notify(ctx, chatID, FormatCheapestWindow(w, tz, sub.Duration))
}

// ---- helpers --------------------------------------------------------------

func parseHHMM(s string) (time.Time, error) {
	t, err := time.Parse("15:04", strings.TrimSpace(s))
	if err != nil {
		return time.Time{}, fmt.Errorf("%w: %q", ErrBadTime, s)
	}
	return t, nil
}

// FormatChargeDispatch composes the overnight-charging message text.
func FormatChargeDispatch(p storage.ChargePlan, w agile.Window, tz *time.Location) string {
	return fmt.Sprintf(
		"Charge plan #%d (%s for %s): start at %s, finish by %s. Mean %.2f p/kWh inc VAT.",
		p.ID, humanDuration(p.Duration),
		p.WindowStartLocal+"–"+p.WindowEndLocal,
		w.Start.In(tz).Format("Mon 02 Jan 15:04"),
		w.End.In(tz).Format("Mon 02 Jan 15:04"),
		w.MeanIncVAT,
	)
}

// FormatCheapestWindow composes the /cheapest and /subscribe reply text.
func FormatCheapestWindow(w agile.Window, tz *time.Location, duration time.Duration) string {
	return fmt.Sprintf(
		"Cheapest %s window: %s → %s (mean %.2f p/kWh inc VAT).",
		humanDuration(duration),
		w.Start.In(tz).Format("Mon 02 Jan 15:04"),
		w.End.In(tz).Format("Mon 02 Jan 15:04"),
		w.MeanIncVAT,
	)
}

func humanDuration(d time.Duration) string {
	d = agile.RoundUpToSlot(d)
	h := int(d / time.Hour)
	m := int((d % time.Hour) / time.Minute)
	switch {
	case h == 0:
		return fmt.Sprintf("%dm", m)
	case m == 0:
		return fmt.Sprintf("%dh", h)
	default:
		return fmt.Sprintf("%dh%02dm", h, m)
	}
}
