package storage

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/zp-bots-telegram/octopus-agile-bot/internal/agile"
)

func newStore(t *testing.T) *Store {
	t.Helper()
	path := filepath.Join(t.TempDir(), "test.db")
	s, err := Open(context.Background(), path)
	require.NoError(t, err)
	t.Cleanup(func() { _ = s.Close() })
	return s
}

func TestMigrationsAreIdempotent(t *testing.T) {
	path := filepath.Join(t.TempDir(), "t.db")
	s1, err := Open(context.Background(), path)
	require.NoError(t, err)
	require.NoError(t, s1.Close())

	s2, err := Open(context.Background(), path)
	require.NoError(t, err)
	require.NoError(t, s2.Close())
}

func TestChatCRUD(t *testing.T) {
	s := newStore(t)
	ctx := context.Background()

	_, ok, err := s.GetChat(ctx, 42)
	require.NoError(t, err)
	assert.False(t, ok)

	require.NoError(t, s.UpsertChatRegion(ctx, 42, "C"))
	got, ok, err := s.GetChat(ctx, 42)
	require.NoError(t, err)
	require.True(t, ok)
	assert.Equal(t, "C", got.Region)
	assert.Equal(t, "Europe/London", got.Timezone)

	require.NoError(t, s.UpsertChatRegion(ctx, 42, "H"))
	got, _, _ = s.GetChat(ctx, 42)
	assert.Equal(t, "H", got.Region)

	require.NoError(t, s.SetChatTimezone(ctx, 42, "Europe/Dublin"))
	got, _, _ = s.GetChat(ctx, 42)
	assert.Equal(t, "Europe/Dublin", got.Timezone)
}

func TestDistinctRegions(t *testing.T) {
	s := newStore(t)
	ctx := context.Background()
	require.NoError(t, s.UpsertChatRegion(ctx, 1, "C"))
	require.NoError(t, s.UpsertChatRegion(ctx, 2, "H"))
	require.NoError(t, s.UpsertChatRegion(ctx, 3, "C"))
	regs, err := s.DistinctRegions(ctx)
	require.NoError(t, err)
	assert.Equal(t, []string{"C", "H"}, regs)
}

func TestSubscriptionRoundTrip(t *testing.T) {
	s := newStore(t)
	ctx := context.Background()
	require.NoError(t, s.UpsertChatRegion(ctx, 7, "C"))

	sub := Subscription{ChatID: 7, Duration: 3 * time.Hour, NotifyAtLocal: "08:00", Enabled: true}
	require.NoError(t, s.SetSubscription(ctx, sub))

	got, ok, err := s.GetSubscription(ctx, 7)
	require.NoError(t, err)
	require.True(t, ok)
	assert.Equal(t, 3*time.Hour, got.Duration)
	assert.Equal(t, "08:00", got.NotifyAtLocal)
	assert.True(t, got.Enabled)

	// upsert update
	sub.Duration = 4 * time.Hour
	require.NoError(t, s.SetSubscription(ctx, sub))
	got, _, _ = s.GetSubscription(ctx, 7)
	assert.Equal(t, 4*time.Hour, got.Duration)

	all, err := s.ListEnabledSubscriptions(ctx)
	require.NoError(t, err)
	assert.Len(t, all, 1)

	require.NoError(t, s.DeleteSubscription(ctx, 7))
	_, ok, _ = s.GetSubscription(ctx, 7)
	assert.False(t, ok)
}

func TestChargePlanCRUD(t *testing.T) {
	s := newStore(t)
	ctx := context.Background()
	require.NoError(t, s.UpsertChatRegion(ctx, 99, "C"))

	id1, err := s.CreateChargePlan(ctx, ChargePlan{
		ChatID: 99, Duration: 4 * time.Hour,
		WindowStartLocal: "22:00", WindowEndLocal: "07:00",
	})
	require.NoError(t, err)
	id2, err := s.CreateChargePlan(ctx, ChargePlan{
		ChatID: 99, Duration: 2 * time.Hour,
		WindowStartLocal: "13:00", WindowEndLocal: "16:00",
	})
	require.NoError(t, err)
	require.NotEqual(t, id1, id2)

	plans, err := s.ListChargePlans(ctx, 99)
	require.NoError(t, err)
	assert.Len(t, plans, 2)

	all, err := s.ListEnabledChargePlans(ctx)
	require.NoError(t, err)
	assert.Len(t, all, 2)

	ok, err := s.CancelChargePlan(ctx, 99, id1)
	require.NoError(t, err)
	assert.True(t, ok)

	ok, err = s.CancelChargePlan(ctx, 99, 9999)
	require.NoError(t, err)
	assert.False(t, ok)
}

func TestRatesRoundTrip(t *testing.T) {
	s := newStore(t)
	ctx := context.Background()

	start := time.Date(2026, 4, 20, 0, 0, 0, 0, time.UTC)
	rates := make([]agile.HalfHour, 4)
	for i := range rates {
		f := start.Add(time.Duration(i) * agile.Slot)
		rates[i] = agile.HalfHour{
			ValidFrom: f, ValidTo: f.Add(agile.Slot),
			UnitRateExcVAT: float64(i), UnitRateIncVAT: float64(i) * 1.05,
		}
	}
	require.NoError(t, s.UpsertRates(ctx, "C", "E-1R-AGILE-24-10-01-C", rates))

	// Upsert again: should not duplicate.
	require.NoError(t, s.UpsertRates(ctx, "C", "E-1R-AGILE-24-10-01-C", rates))

	got, err := s.Rates(ctx, "C", start, start.Add(3*agile.Slot))
	require.NoError(t, err)
	assert.Len(t, got, 3) // slot 0..2 overlap [start, start+90m); slot 3 excluded
	assert.Equal(t, start, got[0].ValidFrom)
}

func TestDispatchLog_Dedup(t *testing.T) {
	s := newStore(t)
	ctx := context.Background()
	day := time.Date(2026, 4, 20, 0, 0, 0, 0, time.UTC)

	first, err := s.MarkChargePlanDispatched(ctx, 1, 1, day)
	require.NoError(t, err)
	assert.True(t, first)

	second, err := s.MarkChargePlanDispatched(ctx, 1, 1, day)
	require.NoError(t, err)
	assert.False(t, second)

	other, err := s.MarkChargePlanDispatched(ctx, 1, 1, day.AddDate(0, 0, 1))
	require.NoError(t, err)
	assert.True(t, other)
}
