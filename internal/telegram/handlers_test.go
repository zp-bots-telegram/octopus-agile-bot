package telegram

import (
	"context"
	"encoding/json"
	"io"
	"mime"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/go-telegram/bot"
	"github.com/go-telegram/bot/models"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/zp-bots-telegram/octopus-agile-bot/internal/agile"
	"github.com/zp-bots-telegram/octopus-agile-bot/internal/service"
	"github.com/zp-bots-telegram/octopus-agile-bot/internal/storage"
)

// tgRecorder is a minimal Telegram Bot API stand-in: it accepts any POST to
// /bot{token}/{method}, parses the multipart form fields, records what was called,
// and returns {"ok":true}.
type tgRecorder struct {
	mu    sync.Mutex
	calls []tgCall
}

type tgCall struct {
	Method string
	Fields map[string]string
}

func newTGRecorder(t *testing.T) (*httptest.Server, *tgRecorder) {
	rec := &tgRecorder{}
	h := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		parts := strings.Split(strings.TrimPrefix(r.URL.Path, "/"), "/")
		// expects: bot{token}, {method}
		if len(parts) < 2 {
			http.Error(w, "bad path", http.StatusBadRequest)
			return
		}
		method := parts[1]

		fields := map[string]string{}
		if ct := r.Header.Get("Content-Type"); ct != "" {
			mt, params, err := mime.ParseMediaType(ct)
			if err == nil && strings.HasPrefix(mt, "multipart/") {
				mr := multipart.NewReader(r.Body, params["boundary"])
				for {
					p, err := mr.NextPart()
					if err == io.EOF {
						break
					}
					if err != nil {
						break
					}
					b, _ := io.ReadAll(p)
					fields[p.FormName()] = string(b)
					_ = p.Close()
				}
			}
		}

		rec.mu.Lock()
		rec.calls = append(rec.calls, tgCall{Method: method, Fields: fields})
		rec.mu.Unlock()

		w.Header().Set("Content-Type", "application/json")
		// A minimal but valid response for every method we use. sendMessage returns a
		// Message; the bot lib tolerates missing fields via json omitempty on decode.
		_, _ = w.Write([]byte(`{"ok":true,"result":{"message_id":1,"date":0,"chat":{"id":0,"type":"private"}}}`))
	})
	srv := httptest.NewServer(h)
	t.Cleanup(srv.Close)
	return srv, rec
}

func (r *tgRecorder) last() tgCall {
	r.mu.Lock()
	defer r.mu.Unlock()
	if len(r.calls) == 0 {
		return tgCall{}
	}
	return r.calls[len(r.calls)-1]
}

func (r *tgRecorder) count() int {
	r.mu.Lock()
	defer r.mu.Unlock()
	return len(r.calls)
}

type fakeOctopus struct{}

func (fakeOctopus) LatestAgileProduct(context.Context) (service.ProductInfo, error) {
	return service.ProductInfo{Code: "AGILE-X"}, nil
}
func (fakeOctopus) StandardUnitRates(context.Context, string, string, time.Time, time.Time) ([]agile.HalfHour, error) {
	return nil, nil
}
func (fakeOctopus) RegionForPostcode(context.Context, string) (string, error) { return "C", nil }
func (fakeOctopus) AccountWithKey(context.Context, string, string) (service.AccountInfo, error) {
	return service.AccountInfo{}, nil
}

type fixedClock struct{ t time.Time }

func (f fixedClock) Now() time.Time { return f.t }

func buildHarness(t *testing.T) (*bot.Bot, *tgRecorder, *storage.Store, *Handlers) {
	t.Helper()
	srv, rec := newTGRecorder(t)

	b, err := bot.New("TEST",
		bot.WithServerURL(srv.URL),
		bot.WithSkipGetMe(),
		bot.WithNotAsyncHandlers(),
	)
	require.NoError(t, err)

	st, err := storage.Open(context.Background(), filepath.Join(t.TempDir(), "t.db"))
	require.NoError(t, err)
	t.Cleanup(func() { _ = st.Close() })

	tz, _ := time.LoadLocation("Europe/London")
	svc := service.New(service.Deps{
		Chats: st, Subs: st, Plans: st, Rates: st,
		Octopus:       fakeOctopus{},
		Notifier:      NewNotifier(b),
		DefaultTZ:     tz,
		DefaultRegion: "C",
		Clock:         fixedClock{t: time.Date(2026, 4, 20, 12, 0, 0, 0, time.UTC)},
	})
	h := NewHandlers(svc, nil, nil)
	h.Register(b)
	return b, rec, st, h
}

func fireText(b *bot.Bot, chatID int64, text string) {
	// go-telegram/bot's MatchTypeCommand only matches if a BotCommand entity points at
	// the command substring — synthesise one spanning the first /token.
	var entities []models.MessageEntity
	if strings.HasPrefix(text, "/") {
		end := strings.IndexByte(text, ' ')
		if end < 0 {
			end = len(text)
		}
		entities = []models.MessageEntity{{
			Type:   models.MessageEntityTypeBotCommand,
			Offset: 0,
			Length: end,
		}}
	}
	b.ProcessUpdate(context.Background(), &models.Update{
		Message: &models.Message{
			ID:       1,
			Chat:     models.Chat{ID: chatID, Type: models.ChatTypePrivate},
			Text:     text,
			Entities: entities,
		},
	})
}

func TestGroupChatCommandWithAtSuffix(t *testing.T) {
	b, rec, _, _ := buildHarness(t)

	text := "/help@octopus_energy_info_bot"
	b.ProcessUpdate(context.Background(), &models.Update{
		Message: &models.Message{
			ID:   1,
			Chat: models.Chat{ID: -1001234567890, Type: models.ChatTypeSupergroup},
			Text: text,
			Entities: []models.MessageEntity{{
				Type:   models.MessageEntityTypeBotCommand,
				Offset: 0,
				Length: len(text),
			}},
		},
	})
	assert.Contains(t, rec.last().Fields["text"], "/cheapest")
}

func TestStartHelpRegion(t *testing.T) {
	b, rec, st, _ := buildHarness(t)

	fireText(b, 10, "/start")
	assert.Contains(t, rec.last().Fields["text"], "Octopus Agile")

	fireText(b, 10, "/help")
	assert.Contains(t, rec.last().Fields["text"], "/cheapest")

	fireText(b, 10, "/region c")
	assert.Contains(t, rec.last().Fields["text"], "Region set to C")

	got, ok, err := st.GetChat(context.Background(), 10)
	require.NoError(t, err)
	require.True(t, ok)
	assert.Equal(t, "C", got.Region)
}

func TestCheapestHappyPath(t *testing.T) {
	b, rec, st, _ := buildHarness(t)
	ctx := context.Background()

	// Seed rates: cheapest 1h starts at 13:00 UTC.
	start := time.Date(2026, 4, 20, 12, 0, 0, 0, time.UTC)
	rates := make([]agile.HalfHour, 6)
	for i := range rates {
		f := start.Add(time.Duration(i) * agile.Slot)
		p := 30.0
		if i == 2 || i == 3 {
			p = 5.0
		}
		rates[i] = agile.HalfHour{ValidFrom: f, ValidTo: f.Add(agile.Slot), UnitRateIncVAT: p, UnitRateExcVAT: p / 1.05}
	}
	require.NoError(t, st.UpsertRates(ctx, "C", "E-1R-AGILE-X-C", rates))

	fireText(b, 20, "/cheapest 1h")
	text := rec.last().Fields["text"]
	assert.Contains(t, text, "Cheapest")
	assert.Contains(t, text, "5.00")
}

func TestAllowlistDeniesStranger(t *testing.T) {
	srv, rec := newTGRecorder(t)
	b, err := bot.New("TEST",
		bot.WithServerURL(srv.URL),
		bot.WithSkipGetMe(),
		bot.WithNotAsyncHandlers(),
	)
	require.NoError(t, err)

	st, err := storage.Open(context.Background(), filepath.Join(t.TempDir(), "t.db"))
	require.NoError(t, err)
	t.Cleanup(func() { _ = st.Close() })
	tz, _ := time.LoadLocation("Europe/London")
	svc := service.New(service.Deps{
		Chats: st, Subs: st, Plans: st, Rates: st,
		Octopus: fakeOctopus{}, Notifier: NewNotifier(b),
		DefaultTZ: tz, DefaultRegion: "C",
	})
	h := NewHandlers(svc, func(chatID int64) bool { return chatID == 42 }, nil)
	h.Register(b)

	fireText(b, 99, "/help")
	assert.Contains(t, rec.last().Fields["text"], "isn't open")

	fireText(b, 42, "/help")
	assert.Contains(t, rec.last().Fields["text"], "/cheapest")
}

func TestChargeAndListAndCancel(t *testing.T) {
	b, rec, _, _ := buildHarness(t)

	fireText(b, 77, "/region C")
	fireText(b, 77, "/charge 4h 22:00-07:00")
	text := rec.last().Fields["text"]
	assert.Contains(t, text, "Charge plan")
	assert.Contains(t, text, "22:00")

	fireText(b, 77, "/charges")
	list := rec.last().Fields["text"]
	assert.Contains(t, list, "4h")
	assert.Contains(t, list, "22:00")

	// Extract the ID (always "#1" for the first plan in a fresh store).
	fireText(b, 77, "/cancelcharge 1")
	assert.Contains(t, rec.last().Fields["text"], "Cancelled")
}

func TestSubscribeValidatesTime(t *testing.T) {
	b, rec, _, _ := buildHarness(t)
	fireText(b, 1, "/subscribe 3h 25:00")
	assert.Contains(t, rec.last().Fields["text"], "HH:MM")
}

func TestNextCommand_NoQualifyingRate(t *testing.T) {
	b, rec, _, _ := buildHarness(t)
	fireText(b, 1, "/region C")
	fireText(b, 1, "/next 10")
	// No rates seeded — expect the "no slot" message.
	assert.True(t,
		strings.Contains(rec.last().Fields["text"], "No slot below") ||
			strings.Contains(rec.last().Fields["text"], "No rates available"),
		"got %q", rec.last().Fields["text"])
}

func TestSendsMultipartForm(t *testing.T) {
	// Sanity check: the recorder actually parses fields.
	b, rec, _, _ := buildHarness(t)
	fireText(b, 5, "/help")
	assert.NotEmpty(t, rec.last().Fields["text"])
	_ = json.RawMessage(nil) // keep import
}
