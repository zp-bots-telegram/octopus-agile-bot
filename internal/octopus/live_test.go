//go:build live

package octopus

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Live tests. Gated by the `live` build tag so they never run by accident.
// Require OCTOPUS_API_KEY in the env. Every call is a read-only GET.

func liveKey(t *testing.T) string {
	t.Helper()
	k := os.Getenv("OCTOPUS_API_KEY")
	if k == "" {
		t.Skip("OCTOPUS_API_KEY not set; skipping live test")
	}
	return k
}

func TestLive_LatestAgileProduct(t *testing.T) {
	c := NewClient(liveKey(t))
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	p, err := c.LatestAgileProduct(ctx)
	require.NoError(t, err)
	assert.NotEmpty(t, p.Code, "product code")
	t.Logf("latest agile product: %s (%s, available from %s)", p.Code, p.FullName, p.Available)
}

func TestLive_StandardUnitRates(t *testing.T) {
	c := NewClient(liveKey(t))
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	p, err := c.LatestAgileProduct(ctx)
	require.NoError(t, err)

	// Pull next-24h rates for region C (London).
	from := time.Now().UTC().Truncate(30 * time.Minute)
	to := from.Add(24 * time.Hour)
	tariff := "E-1R-" + p.Code + "-C"

	rates, err := c.StandardUnitRates(ctx, p.Code, tariff, from, to)
	require.NoError(t, err)
	require.NotEmpty(t, rates, "expected at least one half-hour rate")

	t.Logf("got %d half-hours, first: %s @ %.2f p/kWh (incVAT) / %.2f (excVAT)",
		len(rates),
		rates[0].ValidFrom.Format(time.RFC3339),
		rates[0].UnitRateIncVAT, rates[0].UnitRateExcVAT)

	// Sanity: prices should be plausible (-50 p to 150 p is a generous envelope).
	for _, r := range rates {
		assert.False(t, r.ValidTo.Before(r.ValidFrom), "valid_to before valid_from: %+v", r)
		assert.InDelta(t, 50.0, r.UnitRateIncVAT, 200.0, "rate out of plausible envelope")
	}
}
