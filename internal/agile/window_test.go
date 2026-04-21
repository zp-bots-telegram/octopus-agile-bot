package agile

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// mkRates builds a run of `n` contiguous half-hour rates starting at `start`.
// price(i) returns the inc-VAT rate for slot i.
func mkRates(start time.Time, n int, price func(i int) float64) []HalfHour {
	out := make([]HalfHour, n)
	for i := 0; i < n; i++ {
		f := start.Add(time.Duration(i) * Slot)
		out[i] = HalfHour{
			ValidFrom:      f,
			ValidTo:        f.Add(Slot),
			UnitRateIncVAT: price(i),
			UnitRateExcVAT: price(i) / 1.05,
		}
	}
	return out
}

func TestRoundUpToSlot(t *testing.T) {
	cases := []struct {
		in   time.Duration
		want time.Duration
	}{
		{0, Slot},
		{-1 * time.Minute, Slot},
		{1 * time.Minute, Slot},
		{29 * time.Minute, Slot},
		{30 * time.Minute, 30 * time.Minute},
		{31 * time.Minute, 60 * time.Minute},
		{1 * time.Hour, 1 * time.Hour},
		{61 * time.Minute, 90 * time.Minute},
		{4 * time.Hour, 4 * time.Hour},
	}
	for _, c := range cases {
		assert.Equal(t, c.want, RoundUpToSlot(c.in), "in=%v", c.in)
	}
}

func TestCheapestWindow_Basic(t *testing.T) {
	start := time.Date(2026, 4, 20, 0, 0, 0, 0, time.UTC)
	// Cheapest 2h window is slots 6..9 (sum 10+10+10+10)
	prices := []float64{50, 50, 50, 50, 50, 50, 10, 10, 10, 10, 30, 30, 40, 40}
	rates := mkRates(start, len(prices), func(i int) float64 { return prices[i] })

	w, err := CheapestWindow(rates, 2*time.Hour, start, start.Add(time.Duration(len(prices))*Slot))
	require.NoError(t, err)
	assert.Equal(t, start.Add(6*Slot), w.Start)
	assert.Equal(t, start.Add(10*Slot), w.End)
	assert.InDelta(t, 10.0, w.MeanIncVAT, 1e-9)
	assert.Len(t, w.Slots, 4)
}

func TestCheapestWindow_RoundsUpDuration(t *testing.T) {
	start := time.Date(2026, 4, 20, 0, 0, 0, 0, time.UTC)
	rates := mkRates(start, 6, func(i int) float64 { return float64(i + 1) })
	// 31 minutes -> 60 minutes = 2 slots. Cheapest pair is [1, 2].
	w, err := CheapestWindow(rates, 31*time.Minute, start, start.Add(3*time.Hour))
	require.NoError(t, err)
	assert.Len(t, w.Slots, 2)
	assert.Equal(t, start, w.Start)
}

func TestCheapestWindow_TieTakesFirst(t *testing.T) {
	start := time.Date(2026, 4, 20, 0, 0, 0, 0, time.UTC)
	prices := []float64{10, 10, 10, 10, 50, 50}
	rates := mkRates(start, len(prices), func(i int) float64 { return prices[i] })
	w, err := CheapestWindow(rates, 1*time.Hour, start, start.Add(3*time.Hour))
	require.NoError(t, err)
	assert.Equal(t, start, w.Start)
}

func TestCheapestWindow_Errors(t *testing.T) {
	start := time.Date(2026, 4, 20, 0, 0, 0, 0, time.UTC)
	rates := mkRates(start, 4, func(i int) float64 { return 10 })

	_, err := CheapestWindow(rates, 4*time.Hour, start, start.Add(-1*time.Hour))
	assert.ErrorIs(t, err, ErrEmptyRange)

	_, err = CheapestWindow([]HalfHour{}, 1*time.Hour, start, start.Add(1*time.Hour))
	assert.ErrorIs(t, err, ErrNoRates)

	_, err = CheapestWindow(rates, 5*time.Hour, start, start.Add(2*time.Hour))
	assert.ErrorIs(t, err, ErrDurationTooLong)
}

func TestCheapestWindow_GapBreaksWindow(t *testing.T) {
	start := time.Date(2026, 4, 20, 0, 0, 0, 0, time.UTC)
	// Two cheap slots straddling a gap (1 missing half-hour in the middle).
	rates := []HalfHour{
		{ValidFrom: start, ValidTo: start.Add(Slot), UnitRateIncVAT: 10},
		// gap — slot 2 missing
		{ValidFrom: start.Add(2 * Slot), ValidTo: start.Add(3 * Slot), UnitRateIncVAT: 10},
		{ValidFrom: start.Add(3 * Slot), ValidTo: start.Add(4 * Slot), UnitRateIncVAT: 50},
	}
	// A contiguous 1h (2 slots) is only achievable from index 1..2 at mean 30.
	w, err := CheapestWindow(rates, 1*time.Hour, start, start.Add(4*time.Hour))
	require.NoError(t, err)
	assert.InDelta(t, 30.0, w.MeanIncVAT, 1e-9)
	assert.Equal(t, start.Add(2*Slot), w.Start)
}

func TestCheapestWindowInDailyRange_Overnight(t *testing.T) {
	tz, err := time.LoadLocation("Europe/London")
	require.NoError(t, err)

	// Anchor date 20 Apr 2026 (BST: UTC+1). Range 22:00 → 07:00 next day local.
	// Build enough rates to cover local 22:00 → 07:00 of the following day (18 slots).
	from := time.Date(2026, 4, 20, 21, 0, 0, 0, time.UTC) // 22:00 BST
	rates := mkRates(from, 20, func(i int) float64 {
		// cheapest block is slots 6..9 (i.e. 01:00–03:00 local)
		if i >= 6 && i < 10 {
			return 5
		}
		return 25
	})
	startLocal := time.Date(0, 1, 1, 22, 0, 0, 0, tz)
	endLocal := time.Date(0, 1, 1, 7, 0, 0, 0, tz)

	w, err := CheapestWindowInDailyRange(
		rates, 2*time.Hour,
		time.Date(2026, 4, 20, 12, 0, 0, 0, tz),
		startLocal, endLocal, tz,
	)
	require.NoError(t, err)
	assert.InDelta(t, 5.0, w.MeanIncVAT, 1e-9)
	assert.True(t, w.Start.In(tz).Hour() == 1, "start=%v", w.Start.In(tz))
}

func TestCheapestWindowInDailyRange_DSTForward(t *testing.T) {
	tz, err := time.LoadLocation("Europe/London")
	require.NoError(t, err)

	// UK DST 2026 begins 01:00 UTC on 29 March (clocks jump 01:00 local → 02:00 local).
	// Anchor date 28 March 2026, range 22:00 → 07:00 = 8 local hours (one hour skipped).
	// Build rates covering the entire UTC span 22:00 UTC 28 Mar → 06:00 UTC 29 Mar (17 slots).
	from := time.Date(2026, 3, 28, 22, 0, 0, 0, time.UTC)
	rates := mkRates(from, 17, func(i int) float64 { return 20 })

	_, err = CheapestWindowInDailyRange(
		rates, 3*time.Hour,
		time.Date(2026, 3, 28, 12, 0, 0, 0, tz),
		time.Date(0, 1, 1, 22, 0, 0, 0, tz),
		time.Date(0, 1, 1, 7, 0, 0, 0, tz),
		tz,
	)
	require.NoError(t, err)
}

func TestNextBelowThreshold(t *testing.T) {
	start := time.Date(2026, 4, 20, 0, 0, 0, 0, time.UTC)
	prices := []float64{30, 30, 10, 20, 5}
	rates := mkRates(start, len(prices), func(i int) float64 { return prices[i] })

	got, err := NextBelowThreshold(rates, 15, start)
	require.NoError(t, err)
	assert.Equal(t, start.Add(2*Slot), got.ValidFrom) // first <15 is price 10

	// now skips the 10; next <15 is the 5.
	got, err = NextBelowThreshold(rates, 15, start.Add(3*Slot))
	require.NoError(t, err)
	assert.InDelta(t, 5.0, got.UnitRateIncVAT, 1e-9)

	_, err = NextBelowThreshold(rates, 1, start)
	assert.ErrorIs(t, err, ErrNoRates)
}

func TestTariffCode(t *testing.T) {
	assert.Equal(t, "E-1R-AGILE-24-10-01-C", TariffCode("AGILE-24-10-01", "C"))
}
