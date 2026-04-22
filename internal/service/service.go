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

type OctopusLinkRepo interface {
	SetOctopusLink(ctx context.Context, chatID int64, link storage.OctopusLink) error
}

// Cipher is the minimal encryption surface the service needs — implemented by
// internal/cryptobox in prod, by a passthrough fake in tests.
type Cipher interface {
	Encrypt(plaintext []byte) ([]byte, error)
	Decrypt(ciphertext []byte) ([]byte, error)
}

type PriceAlertRepo interface {
	SetPriceAlert(ctx context.Context, a storage.PriceAlert) error
	DeletePriceAlert(ctx context.Context, chatID int64) error
	GetPriceAlert(ctx context.Context, chatID int64) (storage.PriceAlert, bool, error)
	ListEnabledPriceAlerts(ctx context.Context) ([]storage.PriceAlert, error)
	MarkPriceAlertDispatched(ctx context.Context, chatID int64, runStart time.Time) (bool, error)
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
	// AccountWithKey fetches /v1/accounts/{number}/ authenticated with the supplied
	// per-user API key (not the global one). Used to validate a user's credentials
	// when linking their account.
	AccountWithKey(ctx context.Context, apiKey, accountNumber string) (AccountInfo, error)
	// ConsumptionWithKey fetches the consumption timeseries for one meter authenticated
	// with the supplied per-user API key.
	ConsumptionWithKey(ctx context.Context, apiKey, mpan, meterSerial string, from, to time.Time, groupBy string) ([]ConsumptionPoint, error)
}

// AccountInfo is what the service layer surfaces from an Octopus account. Kept small
// — we only expose what the UI cares about, not the full REST payload.
type AccountInfo struct {
	Number        string
	AddressLine1  string
	Postcode      string
	CurrentTariff string
	MPAN          string
	MeterSerial   string
}

// ConsumptionPoint is the service-layer view of one consumption reading.
type ConsumptionPoint struct {
	IntervalStart time.Time
	IntervalEnd   time.Time
	KWh           float64
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
	alerts    PriceAlertRepo
	links     OctopusLinkRepo
	cipher    Cipher
	octopus   OctopusClient
	notifier  Notifier
	log       *slog.Logger
	clock     Clock
	defaultTZ *time.Location
	defaultRg string
}

type Deps struct {
	Chats        ChatRepo
	Subs         SubscriptionRepo
	Plans        ChargePlanRepo
	Rates        RateRepo
	PriceAlerts  PriceAlertRepo
	OctopusLinks OctopusLinkRepo
	Cipher       Cipher
	Octopus      OctopusClient
	Notifier     Notifier
	Log          *slog.Logger
	Clock        Clock
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
		alerts:    d.PriceAlerts,
		links:     d.OctopusLinks,
		cipher:    d.Cipher,
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
	if err := s.chats.UpsertChatRegion(ctx, chatID, r); err != nil {
		return err
	}
	// Prime the cache for the new region so /cheapest and the chart work immediately.
	// Failures are logged but not propagated — the region change itself succeeded.
	if err := s.refreshRegion(ctx, r); err != nil {
		s.log.Warn("region refresh failed", "region", r, "err", err)
	}
	return nil
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

// refreshRegion fetches and upserts Agile rates for a single region — but only if the
// cache is stale. We skip the Octopus call when we already hold rates for this
// region that cover at least the current window and the next hour (i.e. there's
// something forward-looking for the user to see). The daily scheduled refresh
// backfills beyond this minimum.
func (s *Service) refreshRegion(ctx context.Context, region string) error {
	now := s.clock.Now().UTC()
	have, err := s.rates.Rates(ctx, region, now, now.Add(1*time.Hour))
	if err != nil {
		return fmt.Errorf("check cache: %w", err)
	}
	if len(have) > 0 {
		s.log.Debug("region cache already warm", "region", region, "have", len(have))
		return nil
	}

	prod, err := s.octopus.LatestAgileProduct(ctx)
	if err != nil {
		return fmt.Errorf("latest agile product: %w", err)
	}
	from := now.Truncate(30 * time.Minute)
	to := from.Add(48 * time.Hour)
	tc := agile.TariffCode(prod.Code, region)
	rates, err := s.octopus.StandardUnitRates(ctx, prod.Code, tc, from, to)
	if err != nil {
		return fmt.Errorf("rates: %w", err)
	}
	if err := s.rates.UpsertRates(ctx, region, tc, rates); err != nil {
		return fmt.Errorf("persist: %w", err)
	}
	s.log.Info("region cache warmed", "region", region, "count", len(rates))
	return nil
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

// Rates returns the cached half-hour rates for the chat's region over [from, to].
// Useful for rendering charts / tables — distinct from CheapestWindow which picks one.
func (s *Service) Rates(ctx context.Context, chatID int64, from, to time.Time) ([]agile.HalfHour, error) {
	chat, err := s.resolveChat(ctx, chatID)
	if err != nil {
		return nil, err
	}
	return s.rates.Rates(ctx, chat.Region, from, to)
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
	PriceAlert   *storage.PriceAlert
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
	if s.alerts != nil {
		alert, hasAlert, err := s.alerts.GetPriceAlert(ctx, chatID)
		if err != nil {
			return Status{}, err
		}
		if hasAlert {
			st.PriceAlert = &alert
		}
	}
	return st, nil
}

// ---- price alerts --------------------------------------------------------

// SetPriceAlert turns the per-chat "notify when price drops below threshold" flag on.
// Threshold is inc-VAT p/kWh; use 0 for "only when negative".
func (s *Service) SetPriceAlert(ctx context.Context, chatID int64, thresholdIncVAT float64) error {
	return s.alerts.SetPriceAlert(ctx, storage.PriceAlert{
		ChatID:          chatID,
		ThresholdIncVAT: thresholdIncVAT,
		Enabled:         true,
	})
}

func (s *Service) DisablePriceAlert(ctx context.Context, chatID int64) error {
	return s.alerts.DeletePriceAlert(ctx, chatID)
}

// ContiguousRun is one run of half-hours all strictly below some threshold.
type ContiguousRun struct {
	Start      time.Time
	End        time.Time
	Slots      int
	MinIncVAT  float64
	MeanIncVAT float64
}

// contiguousRunsBelow finds every run of half-hours whose inc-VAT rate is strictly
// less than threshold. Rates must be sorted by ValidFrom ascending; a gap (next slot's
// ValidFrom != previous ValidTo) breaks the run.
func contiguousRunsBelow(rates []agile.HalfHour, threshold float64) []ContiguousRun {
	var out []ContiguousRun
	var cur *ContiguousRun
	var sum float64
	for i, r := range rates {
		if r.UnitRateIncVAT < threshold {
			cont := i > 0 && rates[i-1].ValidTo.Equal(r.ValidFrom) && cur != nil
			if !cont {
				if cur != nil {
					cur.MeanIncVAT = sum / float64(cur.Slots)
					out = append(out, *cur)
				}
				cur = &ContiguousRun{Start: r.ValidFrom, End: r.ValidTo, MinIncVAT: r.UnitRateIncVAT}
				sum = 0
			}
			cur.End = r.ValidTo
			cur.Slots++
			sum += r.UnitRateIncVAT
			if r.UnitRateIncVAT < cur.MinIncVAT {
				cur.MinIncVAT = r.UnitRateIncVAT
			}
		} else if cur != nil {
			cur.MeanIncVAT = sum / float64(cur.Slots)
			out = append(out, *cur)
			cur = nil
			sum = 0
		}
	}
	if cur != nil {
		cur.MeanIncVAT = sum / float64(cur.Slots)
		out = append(out, *cur)
	}
	return out
}

// PriceAlertDispatch is the return from DispatchPriceAlerts — one per message sent.
type PriceAlertDispatch struct {
	ChatID int64
	Run    ContiguousRun
}

// leadStartWindow / leadEndWindow define the "send roughly 10 minutes before" window.
// The dispatcher is expected to be invoked on a short cadence (~1m); any run starting
// inside this band is notified. Picked wider than 10m so a brief outage doesn't cause
// a miss.
const (
	leadWindowMin = 9 * time.Minute
	leadWindowMax = 12 * time.Minute
)

// DispatchPriceAlerts scans every enabled alert. For each, it finds contiguous runs of
// upcoming rates below the user's threshold; if any such run starts inside the lead
// window ([now+9m, now+12m]) AND hasn't been dispatched before, it sends a message.
func (s *Service) DispatchPriceAlerts(ctx context.Context) ([]PriceAlertDispatch, error) {
	if s.alerts == nil {
		return nil, nil
	}
	alerts, err := s.alerts.ListEnabledPriceAlerts(ctx)
	if err != nil {
		return nil, err
	}
	if len(alerts) == 0 {
		return nil, nil
	}

	now := s.clock.Now().UTC()
	lookAhead := now.Add(leadWindowMax + 30*time.Minute)
	leadStart := now.Add(leadWindowMin)
	leadEnd := now.Add(leadWindowMax)

	var out []PriceAlertDispatch
	for _, a := range alerts {
		chat, ok, err := s.chats.GetChat(ctx, a.ChatID)
		if err != nil {
			s.log.Error("price alert: get chat", "chat_id", a.ChatID, "err", err)
			continue
		}
		if !ok || chat.Region == "" {
			continue
		}
		rates, err := s.rates.Rates(ctx, chat.Region, now, lookAhead)
		if err != nil {
			s.log.Error("price alert: rates", "chat_id", a.ChatID, "err", err)
			continue
		}
		runs := contiguousRunsBelow(rates, a.ThresholdIncVAT)
		for _, r := range runs {
			if r.Start.Before(leadStart) || r.Start.After(leadEnd) {
				continue
			}
			fresh, err := s.alerts.MarkPriceAlertDispatched(ctx, a.ChatID, r.Start)
			if err != nil {
				s.log.Error("price alert: mark dispatched", "err", err)
				continue
			}
			if !fresh {
				continue
			}
			tz, err := time.LoadLocation(chat.Timezone)
			if err != nil {
				tz = s.defaultTZ
			}
			if err := s.notifier.Notify(ctx, a.ChatID, FormatPriceAlert(r, a.ThresholdIncVAT, tz)); err != nil {
				s.log.Error("price alert: notify", "chat_id", a.ChatID, "err", err)
				continue
			}
			out = append(out, PriceAlertDispatch{ChatID: a.ChatID, Run: r})
		}
	}
	return out, nil
}

// ---- octopus account link ------------------------------------------------

var (
	ErrLinkNotConfigured = errors.New("octopus account linking is not configured (missing ENCRYPTION_KEY)")
	ErrLinkInvalid       = errors.New("octopus account validation failed — check account number and API key")
)

// LinkedAccount is the read-safe view of a chat's Octopus link — no key material.
type LinkedAccount struct {
	AccountNumber string
	Info          AccountInfo
	Linked        bool
}

// LinkOctopusAccount validates (account_number, api_key) by hitting /v1/accounts/...,
// then encrypts and stores the key on the chat row.
func (s *Service) LinkOctopusAccount(ctx context.Context, chatID int64, accountNumber, apiKey string) (AccountInfo, error) {
	if s.cipher == nil || s.links == nil {
		return AccountInfo{}, ErrLinkNotConfigured
	}
	if strings.TrimSpace(accountNumber) == "" || strings.TrimSpace(apiKey) == "" {
		return AccountInfo{}, fmt.Errorf("%w: account number and api key are both required", ErrLinkInvalid)
	}
	// Make sure the chat row exists (UpsertChatRegion is a no-op on the region if the
	// chat is new — we use the default region as a placeholder until the user sets
	// one). This is a safety net for web-only flows where the chat was created by
	// the login callback but no region is set yet.
	if chat, ok, err := s.chats.GetChat(ctx, chatID); err != nil {
		return AccountInfo{}, err
	} else if !ok {
		if s.defaultRg == "" {
			return AccountInfo{}, ErrNoChat
		}
		if err := s.chats.UpsertChatRegion(ctx, chatID, s.defaultRg); err != nil {
			return AccountInfo{}, err
		}
		_ = chat
	}

	info, err := s.octopus.AccountWithKey(ctx, apiKey, accountNumber)
	if err != nil {
		return AccountInfo{}, fmt.Errorf("%w: %v", ErrLinkInvalid, err)
	}

	ct, err := s.cipher.Encrypt([]byte(apiKey))
	if err != nil {
		return AccountInfo{}, fmt.Errorf("encrypt: %w", err)
	}
	if err := s.links.SetOctopusLink(ctx, chatID, storage.OctopusLink{
		AccountNumber: accountNumber,
		KeyCiphertext: ct,
		MPAN:          info.MPAN,
		MeterSerial:   info.MeterSerial,
	}); err != nil {
		return AccountInfo{}, err
	}
	return info, nil
}

// UnlinkOctopusAccount clears the stored account number + key for the chat.
func (s *Service) UnlinkOctopusAccount(ctx context.Context, chatID int64) error {
	if s.links == nil {
		return ErrLinkNotConfigured
	}
	return s.links.SetOctopusLink(ctx, chatID, storage.OctopusLink{})
}

// Consumption returns per-interval electricity usage for the chat's linked meter over
// [from, to]. groupBy may be "" (half-hourly), "hour", "day", "week", "month", "quarter".
func (s *Service) Consumption(
	ctx context.Context, chatID int64, from, to time.Time, groupBy string,
) ([]ConsumptionPoint, error) {
	if s.cipher == nil {
		return nil, ErrLinkNotConfigured
	}
	chat, ok, err := s.chats.GetChat(ctx, chatID)
	if err != nil {
		return nil, err
	}
	if !ok || len(chat.OctopusAPIKeyCiphertext) == 0 || chat.OctopusMPAN == "" || chat.OctopusMeterSerial == "" {
		return nil, fmt.Errorf("%w: link your octopus account first", ErrLinkInvalid)
	}
	plaintext, err := s.cipher.Decrypt(chat.OctopusAPIKeyCiphertext)
	if err != nil {
		return nil, fmt.Errorf("decrypt api key: %w", err)
	}
	return s.octopus.ConsumptionWithKey(ctx, string(plaintext),
		chat.OctopusMPAN, chat.OctopusMeterSerial, from, to, groupBy)
}

// LinkedAccountFor returns the current link state — never includes the decrypted key.
func (s *Service) LinkedAccountFor(ctx context.Context, chatID int64) (LinkedAccount, error) {
	chat, ok, err := s.chats.GetChat(ctx, chatID)
	if err != nil {
		return LinkedAccount{}, err
	}
	if !ok || chat.OctopusAccountNumber == "" {
		return LinkedAccount{Linked: false}, nil
	}
	return LinkedAccount{Linked: true, AccountNumber: chat.OctopusAccountNumber}, nil
}

// FormatPriceAlert composes the per-alert message.
func FormatPriceAlert(r ContiguousRun, threshold float64, tz *time.Location) string {
	tag := fmt.Sprintf("below %.2f p/kWh", threshold)
	if threshold <= 0 {
		tag = "negative!"
	}
	return fmt.Sprintf(
		"⚡ Prices going %s in ~10 min: %s → %s (%d × 30m, min %.2f p/kWh, mean %.2f p/kWh inc VAT)",
		tag,
		r.Start.In(tz).Format("Mon 15:04"),
		r.End.In(tz).Format("15:04"),
		r.Slots, r.MinIncVAT, r.MeanIncVAT,
	)
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
