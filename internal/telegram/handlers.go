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

// Register wires every command on the bot. Call after bot.New, before bot.Start.
func (h *Handlers) Register(b *bot.Bot) {
	type cmd struct {
		pat string
		fn  bot.HandlerFunc
	}
	// MatchTypeCommand strips the leading slash before comparing — patterns must NOT
	// start with "/".
	cmds := []cmd{
		{"start", h.Start},
		{"help", h.Help},
		{"region", h.Region},
		{"cheapest", h.Cheapest},
		{"next", h.Next},
		{"subscribe", h.Subscribe},
		{"unsubscribe", h.Unsubscribe},
		{"charge", h.Charge},
		{"charges", h.Charges},
		{"cancelcharge", h.CancelCharge},
		{"status", h.StatusCmd},
	}
	for _, c := range cmds {
		b.RegisterHandler(bot.HandlerTypeMessageText, c.pat, bot.MatchTypeCommand, c.fn)
	}
}

// ---- command impls --------------------------------------------------------

func (h *Handlers) Start(ctx context.Context, b *bot.Bot, u *models.Update) {
	if !h.gate(ctx, b, u) {
		return
	}
	h.reply(ctx, b, u,
		"Hi! I help you time high-load appliances against Octopus Agile.\n"+
			"Start with /region <letter> (A–P) to pick your DNO region, then try /cheapest 3h.\n"+
			"Use /help for the full command list.")
}

func (h *Handlers) Help(ctx context.Context, b *bot.Bot, u *models.Update) {
	if !h.gate(ctx, b, u) {
		return
	}
	h.reply(ctx, b, u, `Commands:
/region <A-P> — set your DNO region
/cheapest <duration> — cheapest window in the next 24–48h, e.g. /cheapest 3h
/next <p> — next half-hour below <p> p/kWh, e.g. /next 15
/subscribe <duration> <HH:MM> — daily notification of the cheapest window
/unsubscribe
/charge <duration> <HH:MM>-<HH:MM> — daily EV-charge plan, e.g. /charge 4h 22:00-07:00
/charges — list active charge plans
/cancelcharge <id>
/status — show your settings`)
}

func (h *Handlers) Region(ctx context.Context, b *bot.Bot, u *models.Update) {
	if !h.gate(ctx, b, u) {
		return
	}
	args := argsOf(u.Message)
	if len(args) != 1 {
		h.reply(ctx, b, u, "Usage: /region <letter A-P>")
		return
	}
	if err := h.svc.SetRegion(ctx, u.Message.Chat.ID, args[0]); err != nil {
		h.reply(ctx, b, u, friendly(err))
		return
	}
	h.reply(ctx, b, u, "Region set to "+strings.ToUpper(args[0])+".")
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
	fmt.Fprintf(&sb, "Region: %s\nTimezone: %s\n", st.Region, st.Timezone)
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
