// Package telegram is the Telegram transport adapter. Handlers are thin: they parse
// the command + args, call the service layer, and format the reply. No business logic.
package telegram

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"strconv"
	"strings"
	"time"

	"github.com/go-telegram/bot"
	"github.com/go-telegram/bot/models"
	"github.com/zp-bots-telegram/octopus-agile-bot/internal/agile"
	"github.com/zp-bots-telegram/octopus-agile-bot/internal/service"
)

// AllowedFunc is the chat-ID allowlist predicate. Return true to permit.
type AllowedFunc func(chatID int64) bool

type Handlers struct {
	svc     *service.Service
	allowed AllowedFunc
	log     *slog.Logger
}

func NewHandlers(svc *service.Service, allowed AllowedFunc, log *slog.Logger) *Handlers {
	if allowed == nil {
		allowed = func(int64) bool { return true }
	}
	if log == nil {
		log = slog.Default()
	}
	return &Handlers{svc: svc, allowed: allowed, log: log}
}

// commandSpec is the single source of truth for every v1 command: the pattern the
// handler matches, the function to invoke, and the description shown in the Telegram
// client's command menu.
type commandSpec struct {
	name        string
	description string
	fn          bot.HandlerFunc
}

func (h *Handlers) specs() []commandSpec {
	return []commandSpec{
		{"start", "Intro and setup", h.Start},
		{"help", "List commands", h.Help},
		{"region", "Set region: /region C or /region SW1A1AA", h.Region},
		{"cheapest", "Cheapest window, e.g. /cheapest 3h", h.Cheapest},
		{"next", "Next slot below price, e.g. /next 15", h.Next},
		{"subscribe", "Daily cheapest-window push, e.g. /subscribe 3h 08:00", h.Subscribe},
		{"unsubscribe", "Stop daily cheapest-window push", h.Unsubscribe},
		{"charge", "Recurring EV charge plan, e.g. /charge 4h 22:00-07:00", h.Charge},
		{"charges", "List active charge plans", h.Charges},
		{"cancelcharge", "Cancel a charge plan by id", h.CancelCharge},
		{"alerts", "Alert me when prices go low, e.g. /alerts 0 or /alerts off", h.Alerts},
		{"status", "Show your settings", h.StatusCmd},
	}
}

// Register wires every command on the bot. Call after bot.New, before bot.Start.
// We use a custom match function instead of MatchTypeCommand so we can handle the
// group-chat form `/cmd@botusername`, which the built-in matcher rejects because it
// compares the whole command token including the `@suffix`.
func (h *Handlers) Register(b *bot.Bot) {
	for _, s := range h.specs() {
		b.RegisterHandlerMatchFunc(matchCommand(s.name), s.fn)
	}
}

// matchCommand matches a message containing a BotCommand entity whose token (minus
// any `@botusername` suffix) equals `pattern`.
func matchCommand(pattern string) bot.MatchFunc {
	return func(update *models.Update) bool {
		if update.Message == nil {
			return false
		}
		text := update.Message.Text
		for _, e := range update.Message.Entities {
			if e.Type != models.MessageEntityTypeBotCommand {
				continue
			}
			if e.Offset+e.Length > len(text) {
				continue
			}
			// Entity spans "/cmd" or "/cmd@botusername"; drop the leading slash.
			token := text[e.Offset+1 : e.Offset+e.Length]
			if at := strings.IndexByte(token, '@'); at >= 0 {
				token = token[:at]
			}
			if token == pattern {
				return true
			}
		}
		return false
	}
}

// PublishCommands pushes the command menu to Telegram via setMyCommands. Call once
// at startup so the UI matches the code without a manual curl.
func (h *Handlers) PublishCommands(ctx context.Context, b *bot.Bot) error {
	specs := h.specs()
	cmds := make([]models.BotCommand, 0, len(specs))
	for _, s := range specs {
		cmds = append(cmds, models.BotCommand{Command: s.name, Description: s.description})
	}
	_, err := b.SetMyCommands(ctx, &bot.SetMyCommandsParams{Commands: cmds})
	return err
}

// ---- command impls --------------------------------------------------------

func (h *Handlers) Start(ctx context.Context, b *bot.Bot, u *models.Update) {
	if !h.gate(ctx, b, u) {
		return
	}
	h.reply(ctx, b, u,
		"Hi! I help you time high-load appliances (EV charging, dishwasher, etc.) against Octopus Agile's half-hourly prices.\n\n"+
			"Start by telling me your DNO region with either:\n"+
			"  • /region <postcode>  — e.g. /region SW1A 1AA (I'll look up the letter)\n"+
			"  • /region <letter>     — e.g. /region C\n\n"+
			"Then try:\n"+
			"  /cheapest 3h — best 3-hour window in the next 24–48h\n"+
			"  /charge 4h 22:00-07:00 — daily EV charge plan, I'll DM you when to plug in\n"+
			"  /next 15 — the next half-hour under 15 p/kWh\n\n"+
			"Full list with /help.")
}

func (h *Handlers) Help(ctx context.Context, b *bot.Bot, u *models.Update) {
	if !h.gate(ctx, b, u) {
		return
	}
	h.reply(ctx, b, u, `Commands:
/region <A-P> or <postcode> — set your DNO region
/cheapest <duration> — cheapest window in the next 24–48h, e.g. /cheapest 3h
/next <p> — next half-hour below <p> p/kWh, e.g. /next 15
/subscribe <duration> <HH:MM> — daily notification of the cheapest window
/unsubscribe
/charge <duration> <HH:MM>-<HH:MM> — daily EV-charge plan, e.g. /charge 4h 22:00-07:00
/charges — list active charge plans
/cancelcharge <id>
/alerts <threshold|off> — notify ~10 min before prices go below <threshold> p/kWh (default 0 = negative only)
/status — show your settings`)
}

func (h *Handlers) Region(ctx context.Context, b *bot.Bot, u *models.Update) {
	if !h.gate(ctx, b, u) {
		return
	}
	args := argsOf(u.Message)
	if len(args) == 0 {
		h.reply(ctx, b, u, "Usage: /region <letter A-P> or /region <postcode>")
		return
	}
	// Join args so postcodes with spaces ("SW1A 1AA") still work.
	input := strings.ToUpper(strings.TrimSpace(strings.Join(args, " ")))

	// Single-letter fast path.
	if len(input) == 1 {
		if err := h.svc.SetRegion(ctx, u.Message.Chat.ID, input); err != nil {
			h.reply(ctx, b, u, friendly(err))
			return
		}
		h.reply(ctx, b, u, "Region set to "+formatRegion(input)+".")
		return
	}

	// Otherwise treat as postcode.
	r, err := h.svc.SetRegionByPostcode(ctx, u.Message.Chat.ID, input)
	if err != nil {
		h.reply(ctx, b, u, "Couldn't resolve postcode "+input+": "+err.Error())
		return
	}
	h.reply(ctx, b, u, "Region set to "+formatRegion(r)+" (resolved from postcode "+input+").")
}

// formatRegion returns e.g. `A (Eastern England)` or just `A` if the letter is unknown.
func formatRegion(letter string) string {
	if name := agile.RegionName(letter); name != "" {
		return letter + " (" + name + ")"
	}
	return letter
}

func (h *Handlers) Cheapest(ctx context.Context, b *bot.Bot, u *models.Update) {
	if !h.gate(ctx, b, u) {
		return
	}
	args := argsOf(u.Message)
	if len(args) != 1 {
		h.reply(ctx, b, u, "Usage: /cheapest <duration>, e.g. /cheapest 3h")
		return
	}
	d, err := time.ParseDuration(args[0])
	if err != nil || d <= 0 {
		h.reply(ctx, b, u, "Couldn't parse duration "+args[0])
		return
	}
	w, err := h.svc.CheapestWindow(ctx, u.Message.Chat.ID, d)
	if err != nil {
		h.reply(ctx, b, u, friendly(err))
		return
	}
	h.reply(ctx, b, u, service.FormatCheapestWindow(w, h.chatTZ(ctx, u.Message.Chat.ID), d))
}

func (h *Handlers) Next(ctx context.Context, b *bot.Bot, u *models.Update) {
	if !h.gate(ctx, b, u) {
		return
	}
	args := argsOf(u.Message)
	if len(args) != 1 {
		h.reply(ctx, b, u, "Usage: /next <threshold-p-per-kwh>, e.g. /next 15")
		return
	}
	t, err := strconv.ParseFloat(args[0], 64)
	if err != nil {
		h.reply(ctx, b, u, "Couldn't parse threshold "+args[0])
		return
	}
	hh, err := h.svc.NextBelowThreshold(ctx, u.Message.Chat.ID, t)
	if err != nil {
		if errors.Is(err, agile.ErrNoRates) {
			h.reply(ctx, b, u, fmt.Sprintf("No slot below %.2f p/kWh in the published horizon.", t))
			return
		}
		h.reply(ctx, b, u, friendly(err))
		return
	}
	tz := h.chatTZ(ctx, u.Message.Chat.ID)
	h.reply(ctx, b, u, fmt.Sprintf(
		"Next sub-%.2f slot: %s (%.2f p/kWh).",
		t, hh.ValidFrom.In(tz).Format("Mon 02 Jan 15:04"), hh.UnitRateIncVAT,
	))
}

func (h *Handlers) Subscribe(ctx context.Context, b *bot.Bot, u *models.Update) {
	if !h.gate(ctx, b, u) {
		return
	}
	args := argsOf(u.Message)
	if len(args) != 2 {
		h.reply(ctx, b, u, "Usage: /subscribe <duration> <HH:MM>, e.g. /subscribe 3h 08:00")
		return
	}
	d, err := time.ParseDuration(args[0])
	if err != nil || d <= 0 {
		h.reply(ctx, b, u, "Couldn't parse duration "+args[0])
		return
	}
	if err := h.svc.SetSubscription(ctx, u.Message.Chat.ID, d, args[1]); err != nil {
		h.reply(ctx, b, u, friendly(err))
		return
	}
	h.reply(ctx, b, u, fmt.Sprintf("Subscribed — daily message at %s for a %s window.", args[1], args[0]))
}

func (h *Handlers) Unsubscribe(ctx context.Context, b *bot.Bot, u *models.Update) {
	if !h.gate(ctx, b, u) {
		return
	}
	if err := h.svc.Unsubscribe(ctx, u.Message.Chat.ID); err != nil {
		h.reply(ctx, b, u, friendly(err))
		return
	}
	h.reply(ctx, b, u, "Unsubscribed.")
}

func (h *Handlers) Charge(ctx context.Context, b *bot.Bot, u *models.Update) {
	if !h.gate(ctx, b, u) {
		return
	}
	args := argsOf(u.Message)
	if len(args) != 2 {
		h.reply(ctx, b, u, "Usage: /charge <duration> <HH:MM>-<HH:MM>, e.g. /charge 4h 22:00-07:00")
		return
	}
	d, err := time.ParseDuration(args[0])
	if err != nil || d <= 0 {
		h.reply(ctx, b, u, "Couldn't parse duration "+args[0])
		return
	}
	start, end, ok := strings.Cut(args[1], "-")
	if !ok {
		h.reply(ctx, b, u, "Window must be HH:MM-HH:MM")
		return
	}
	p, err := h.svc.CreateChargePlan(ctx, u.Message.Chat.ID, d, start, end)
	if err != nil {
		h.reply(ctx, b, u, friendly(err))
		return
	}
	h.reply(ctx, b, u, fmt.Sprintf(
		"Charge plan #%d saved: %s per day, allowed %s–%s. I'll message you each afternoon once tomorrow's rates land.",
		p.ID, args[0], start, end,
	))
}

func (h *Handlers) Charges(ctx context.Context, b *bot.Bot, u *models.Update) {
	if !h.gate(ctx, b, u) {
		return
	}
	plans, err := h.svc.ListChargePlans(ctx, u.Message.Chat.ID)
	if err != nil {
		h.reply(ctx, b, u, friendly(err))
		return
	}
	if len(plans) == 0 {
		h.reply(ctx, b, u, "No charge plans. Add one with /charge.")
		return
	}
	var sb strings.Builder
	sb.WriteString("Active charge plans:\n")
	for _, p := range plans {
		fmt.Fprintf(&sb, "#%d — %s per day, %s–%s\n",
			p.ID, roundedDur(p.Duration), p.WindowStartLocal, p.WindowEndLocal)
	}
	h.reply(ctx, b, u, strings.TrimRight(sb.String(), "\n"))
}

func (h *Handlers) CancelCharge(ctx context.Context, b *bot.Bot, u *models.Update) {
	if !h.gate(ctx, b, u) {
		return
	}
	args := argsOf(u.Message)
	if len(args) != 1 {
		h.reply(ctx, b, u, "Usage: /cancelcharge <id>")
		return
	}
	id, err := strconv.ParseInt(args[0], 10, 64)
	if err != nil {
		h.reply(ctx, b, u, "Id must be a number.")
		return
	}
	ok, err := h.svc.CancelChargePlan(ctx, u.Message.Chat.ID, id)
	if err != nil {
		h.reply(ctx, b, u, friendly(err))
		return
	}
	if !ok {
		h.reply(ctx, b, u, fmt.Sprintf("No charge plan #%d for this chat.", id))
		return
	}
	h.reply(ctx, b, u, fmt.Sprintf("Cancelled charge plan #%d.", id))
}

// Alerts lets the user enable/disable the "price dropped below threshold" notification.
// Usage:
//
//	/alerts           → default (threshold 0, i.e. notify on any negative slot)
//	/alerts 15        → notify when a slot goes below 15 p/kWh inc VAT
//	/alerts off       → disable
func (h *Handlers) Alerts(ctx context.Context, b *bot.Bot, u *models.Update) {
	if !h.gate(ctx, b, u) {
		return
	}
	args := argsOf(u.Message)
	if len(args) == 1 && strings.EqualFold(args[0], "off") {
		if err := h.svc.DisablePriceAlert(ctx, u.Message.Chat.ID); err != nil {
			h.reply(ctx, b, u, friendly(err))
			return
		}
		h.reply(ctx, b, u, "Price alerts disabled.")
		return
	}
	threshold := 0.0
	if len(args) == 1 {
		t, err := strconv.ParseFloat(args[0], 64)
		if err != nil {
			h.reply(ctx, b, u, "Usage: /alerts [threshold | off], e.g. /alerts 0 or /alerts 15")
			return
		}
		threshold = t
	}
	if err := h.svc.SetPriceAlert(ctx, u.Message.Chat.ID, threshold); err != nil {
		h.reply(ctx, b, u, friendly(err))
		return
	}
	tag := fmt.Sprintf("below %.2f p/kWh", threshold)
	if threshold <= 0 {
		tag = "go negative"
	}
	h.reply(ctx, b, u, "Alerts on: I'll message you ~10 min before prices "+tag+".")
}

func (h *Handlers) StatusCmd(ctx context.Context, b *bot.Bot, u *models.Update) {
	if !h.gate(ctx, b, u) {
		return
	}
	st, err := h.svc.Status(ctx, u.Message.Chat.ID)
	if err != nil {
		h.reply(ctx, b, u, friendly(err))
		return
	}
	var sb strings.Builder
	fmt.Fprintf(&sb, "Region: %s\nTimezone: %s\n", formatRegion(st.Region), st.Timezone)
	if st.Subscription != nil {
		fmt.Fprintf(&sb, "Subscription: %s at %s\n",
			roundedDur(st.Subscription.Duration), st.Subscription.NotifyAtLocal)
	} else {
		sb.WriteString("Subscription: none\n")
	}
	if len(st.ChargePlans) == 0 {
		sb.WriteString("Charge plans: none\n")
	} else {
		sb.WriteString("Charge plans:\n")
		for _, p := range st.ChargePlans {
			fmt.Fprintf(&sb, "  #%d — %s per day, %s–%s\n",
				p.ID, roundedDur(p.Duration), p.WindowStartLocal, p.WindowEndLocal)
		}
	}
	if st.PriceAlert != nil && st.PriceAlert.Enabled {
		if st.PriceAlert.ThresholdIncVAT <= 0 {
			sb.WriteString("Price alert: on (negative prices)\n")
		} else {
			fmt.Fprintf(&sb, "Price alert: on (< %.2f p/kWh)\n", st.PriceAlert.ThresholdIncVAT)
		}
	} else {
		sb.WriteString("Price alert: off\n")
	}
	h.reply(ctx, b, u, strings.TrimRight(sb.String(), "\n"))
}

// ---- helpers --------------------------------------------------------------

// gate enforces the allowlist. Returns true when the update should be processed.
func (h *Handlers) gate(ctx context.Context, b *bot.Bot, u *models.Update) bool {
	if u.Message == nil {
		return false
	}
	if h.allowed(u.Message.Chat.ID) {
		return true
	}
	h.log.Warn("denied chat", "chat_id", u.Message.Chat.ID)
	_, _ = b.SendMessage(ctx, &bot.SendMessageParams{
		ChatID: u.Message.Chat.ID,
		Text:   "Sorry — this bot isn't open to you yet.",
	})
	return false
}

func (h *Handlers) reply(ctx context.Context, b *bot.Bot, u *models.Update, text string) {
	if _, err := b.SendMessage(ctx, &bot.SendMessageParams{
		ChatID: u.Message.Chat.ID,
		Text:   text,
	}); err != nil {
		h.log.Error("send reply", "chat_id", u.Message.Chat.ID, "err", err)
	}
}

// argsOf returns whitespace-split args following the command token.
func argsOf(m *models.Message) []string {
	if m == nil {
		return nil
	}
	parts := strings.Fields(m.Text)
	if len(parts) <= 1 {
		return nil
	}
	return parts[1:]
}

// chatTZ returns the caller's timezone, falling back to UTC if the chat is unknown or
// its timezone is unloadable.
func (h *Handlers) chatTZ(ctx context.Context, chatID int64) *time.Location {
	st, err := h.svc.Status(ctx, chatID)
	if err != nil {
		return time.UTC
	}
	loc, err := time.LoadLocation(st.Timezone)
	if err != nil {
		return time.UTC
	}
	return loc
}

func roundedDur(d time.Duration) string {
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

// friendly maps known sentinel errors to a user-facing message.
func friendly(err error) string {
	switch {
	case errors.Is(err, service.ErrInvalidRegion):
		return "Region must be a single letter A–P."
	case errors.Is(err, service.ErrBadTime):
		return "Time must be in HH:MM 24-hour format, e.g. 08:00."
	case errors.Is(err, service.ErrNoChat):
		return "Set a region first with /region <letter>."
	case errors.Is(err, agile.ErrNoRates):
		return "No rates available yet — try again after the daily refresh."
	case errors.Is(err, agile.ErrDurationTooLong):
		return "That duration is longer than the current rate horizon."
	default:
		return "Sorry, something went wrong: " + err.Error()
	}
}
