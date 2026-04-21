package service

import (
	"context"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/zp-bots-telegram/octopus-agile-bot/internal/agile"
	"github.com/zp-bots-telegram/octopus-agile-bot/internal/storage"
)

type fakeOctopus struct {
	prodCode string
	rates    []agile.HalfHour
	err      error
}

func (f *fakeOctopus) LatestAgileProduct(ctx context.Context) (ProductInfo, error) {
	if f.err != nil {
		return ProductInfo{}, f.err
	}
	return ProductInfo{Code: f.prodCode}, nil
}
func (f *fakeOctopus) StandardUnitRates(ctx context.Context, _, _ string, _, _ time.Time) ([]agile.HalfHour, error) {
	return f.rates, f.err
}

type fakeNotifier struct {
	mu   sync.Mutex
	sent []struct {
		ChatID int64
		Text   string
	}
}

func (f *fakeNotifier) Notify(_ context.Context, chatID int64, text string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.sent = append(f.sent, struct {
		ChatID int64
		Text   string
	}{chatID, text})
	return nil
}

func (f *fakeNotifier) count() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return len(f.sent)
}

type fixedClock struct{ t time.Time }

func (f fixedClock) Now() time.Time { return f.t }

// buildService stitches together a real SQLite store (tested in storage_test.go) with
// the in-memory fakes for side-effecting dependencies.
func buildService(t *testing.T, now time.Time) (*Service, *storage.Store, *fakeOctopus, *fakeNotifier) {
	t.Helper()
	st, err := storage.Open(context.Background(), filepath.Join(t.TempDir(), "t.db"))
	require.NoError(t, err)
	t.Cleanup(func() { _ = st.Close() })

	octo := &fakeOctopus{prodCode: "AGILE-24-10-01"}
	notifier := &fakeNotifier{}
	tz, _ := time.LoadLocation("Europe/London")
	svc := New(Deps{
		Chats: st, Subs: st, Plans: st, Rates: st,
		Octopus: octo, Notifier: notifier,
		Clock:         fixedClock{t: now},
		DefaultTZ:     tz,
		DefaultRegion: "C",
	})
	return svc, st, octo, notifier
}

func buildRates(start time.Time, prices []float64) []agile.HalfHour {
	out := make([]agile.HalfHour, len(prices))
	for i, p := range prices {
		f := start.Add(time.Duration(i) * agile.Slot)
		out[i] = agile.HalfHour{ValidFrom: f, ValidTo: f.Add(agile.Slot), UnitRateIncVAT: p, UnitRateExcVAT: p / 1.05}
	}
	return out
}

func TestSetRegion_Validation(t *testing.T) {
	svc, st, _, _ := buildService(t, time.Now())
	ctx := context.Background()

	require.NoError(t, svc.SetRegion(ctx, 1, " h "))
	c, ok, _ := st.GetChat(ctx, 1)
	require.True(t, ok)
	assert.Equal(t, "H", c.Region)

	err := svc.SetRegion(ctx, 1, "Z")
	assert.ErrorIs(t, err, ErrInvalidRegion)
}

func TestCheapestWindow_UsesDefaultRegionIfChatUnknown(t *testing.T) {
	now := time.Date(2026, 4, 20, 10, 0, 0, 0, time.UTC)
	svc, st, _, _ := buildService(t, now)
	ctx := context.Background()

	// Seed cheap window around 13:00-14:00 UTC.
	rates := buildRates(now, []float64{30, 30, 30, 30, 30, 30, 5, 5, 30, 30})
	require.NoError(t, st.UpsertRates(ctx, "C", "E-1R-AGILE-24-10-01-C", rates))

	w, err := svc.CheapestWindow(ctx, 9999, 1*time.Hour)
	require.NoError(t, err)
	assert.InDelta(t, 5.0, w.MeanIncVAT, 1e-9)
}

func TestNextBelowThreshold_RespectsChatRegion(t *testing.T) {
	now := time.Date(2026, 4, 20, 10, 0, 0, 0, time.UTC)
	svc, st, _, _ := buildService(t, now)
	ctx := context.Background()
	require.NoError(t, svc.SetRegion(ctx, 5, "H"))

	rates := buildRates(now, []float64{20, 20, 5, 20})
	require.NoError(t, st.UpsertRates(ctx, "H", "E-1R-AGILE-24-10-01-H", rates))

	hh, err := svc.NextBelowThreshold(ctx, 5, 10)
	require.NoError(t, err)
	assert.InDelta(t, 5.0, hh.UnitRateIncVAT, 1e-9)
}

func TestSetSubscription_ValidatesTime(t *testing.T) {
	svc, _, _, _ := buildService(t, time.Now())
	err := svc.SetSubscription(context.Background(), 1, 3*time.Hour, "25:00")
	assert.ErrorIs(t, err, ErrBadTime)
}

func TestStatus(t *testing.T) {
	svc, _, _, _ := buildService(t, time.Now())
	ctx := context.Background()

	require.NoError(t, svc.SetRegion(ctx, 7, "C"))
	require.NoError(t, svc.SetSubscription(ctx, 7, 3*time.Hour, "08:00"))
	_, err := svc.CreateChargePlan(ctx, 7, 4*time.Hour, "22:00", "07:00")
	require.NoError(t, err)

	st, err := svc.Status(ctx, 7)
	require.NoError(t, err)
	assert.Equal(t, "C", st.Region)
	require.NotNil(t, st.Subscription)
	assert.Equal(t, "08:00", st.Subscription.NotifyAtLocal)
	assert.Len(t, st.ChargePlans, 1)
}

func TestRefreshRates_PullsPerRegion(t *testing.T) {
	now := time.Date(2026, 4, 20, 16, 15, 0, 0, time.UTC)
	svc, st, octo, _ := buildService(t, now)
	ctx := context.Background()

	require.NoError(t, svc.SetRegion(ctx, 1, "C"))
	require.NoError(t, svc.SetRegion(ctx, 2, "H"))

	octo.rates = buildRates(now, []float64{10, 20, 30, 40})

	n, err := svc.RefreshRates(ctx)
	require.NoError(t, err)
	assert.Equal(t, 8, n) // 4 rates × 2 regions

	for _, r := range []string{"C", "H"} {
		got, err := st.Rates(ctx, r, now, now.Add(2*time.Hour))
		require.NoError(t, err)
		assert.Len(t, got, 4)
	}
}

func TestDispatchChargePlans_SendsOncePerDay(t *testing.T) {
	tz, _ := time.LoadLocation("Europe/London")
	// 16:15 UTC = 17:15 BST on 20 Apr 2026 (BST).
	now := time.Date(2026, 4, 20, 16, 15, 0, 0, time.UTC)
	svc, st, _, notifier := buildService(t, now)
	ctx := context.Background()

	require.NoError(t, svc.SetRegion(ctx, 42, "C"))
	_, err := svc.CreateChargePlan(ctx, 42, 2*time.Hour, "22:00", "07:00")
	require.NoError(t, err)

	// Seed overnight rates (21:00 UTC → 06:00 UTC of 21 Apr; cheapest slot 01:00–03:00 UTC).
	ratesStart := time.Date(2026, 4, 20, 21, 0, 0, 0, time.UTC)
	prices := make([]float64, 18)
	for i := range prices {
		prices[i] = 25
	}
	prices[8] = 5 // 01:00 UTC
	prices[9] = 5
	prices[10] = 5
	prices[11] = 5
	require.NoError(t, st.UpsertRates(ctx, "C", "E-1R-AGILE-24-10-01-C", buildRates(ratesStart, prices)))
	_ = tz

	dispatches, err := svc.DispatchTodaysChargePlans(ctx)
	require.NoError(t, err)
	require.Len(t, dispatches, 1)
	assert.Equal(t, 1, notifier.count())

	// Second call same day: no new sends.
	_, err = svc.DispatchTodaysChargePlans(ctx)
	require.NoError(t, err)
	assert.Equal(t, 1, notifier.count())
}

func TestDispatchSubscription_SendsMessage(t *testing.T) {
	now := time.Date(2026, 4, 20, 7, 0, 0, 0, time.UTC)
	svc, st, _, notifier := buildService(t, now)
	ctx := context.Background()

	require.NoError(t, svc.SetRegion(ctx, 3, "C"))
	require.NoError(t, svc.SetSubscription(ctx, 3, 1*time.Hour, "08:00"))
	require.NoError(t, st.UpsertRates(ctx, "C", "E-1R-AGILE-24-10-01-C",
		buildRates(now, []float64{30, 30, 5, 5, 30})))

	require.NoError(t, svc.DispatchSubscription(ctx, 3))
	assert.Equal(t, 1, notifier.count())
}

func TestHumanDuration(t *testing.T) {
	assert.Equal(t, "30m", humanDuration(30*time.Minute))
	assert.Equal(t, "1h", humanDuration(60*time.Minute))
	assert.Equal(t, "1h30m", humanDuration(90*time.Minute))
	assert.Equal(t, "4h", humanDuration(4*time.Hour))
}
