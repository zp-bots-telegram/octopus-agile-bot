// Package agile contains pure, transport-agnostic domain logic for the Octopus Agile
// tariff: the rate model, the cheapest-window algorithm, and the threshold search.
// It must not import storage, HTTP, Telegram, or any other side-effecting package.
package agile

import (
	"errors"
	"math"
	"sort"
	"time"
)

// Slot is the fixed Agile half-hour settlement period.
const Slot = 30 * time.Minute

// HalfHour is a single half-hour period of Agile pricing.
type HalfHour struct {
	ValidFrom      time.Time
	ValidTo        time.Time
	UnitRateExcVAT float64
	UnitRateIncVAT float64
}

// Window is a contiguous run of HalfHours selected as the "cheapest" slot.
type Window struct {
	Start      time.Time
	End        time.Time
	Slots      []HalfHour
	MeanIncVAT float64 // p/kWh, inc VAT
}

var (
	ErrNoRates         = errors.New("no rates available in range")
	ErrDurationTooLong = errors.New("requested duration longer than available horizon")
	ErrEmptyRange      = errors.New("empty or invalid time range")
)

// RoundUpToSlot rounds a duration up to the nearest 30-minute boundary.
// Zero and sub-slot durations round up to one slot.
func RoundUpToSlot(d time.Duration) time.Duration {
	if d <= 0 {
		return Slot
	}
	if r := d % Slot; r != 0 {
		return d + (Slot - r)
	}
	return d
}

// CheapestWindow finds the cheapest contiguous window of `duration` whose slots all
// fall within [from, to]. Gaps in the data (non-contiguous half-hours) reset the
// sliding window — a cheap window must be physically contiguous.
func CheapestWindow(rates []HalfHour, duration time.Duration, from, to time.Time) (Window, error) {
	if !to.After(from) {
		return Window{}, ErrEmptyRange
	}
	duration = RoundUpToSlot(duration)
	slots := int(duration / Slot)

	filt := make([]HalfHour, 0, len(rates))
	for _, r := range rates {
		if !r.ValidFrom.Before(from) && !r.ValidTo.After(to) {
			filt = append(filt, r)
		}
	}
	if len(filt) == 0 {
		return Window{}, ErrNoRates
	}
	sort.Slice(filt, func(i, j int) bool { return filt[i].ValidFrom.Before(filt[j].ValidFrom) })
	if len(filt) < slots {
		return Window{}, ErrDurationTooLong
	}

	bestStart := -1
	bestSum := math.Inf(1)
	runStart := 0
	var sum float64
	for i := 0; i < len(filt); i++ {
		if i > 0 && !filt[i].ValidFrom.Equal(filt[i-1].ValidTo) {
			runStart = i
			sum = 0
		}
		sum += filt[i].UnitRateIncVAT
		for i-runStart+1 > slots {
			sum -= filt[runStart].UnitRateIncVAT
			runStart++
		}
		if i-runStart+1 == slots && sum < bestSum {
			bestSum = sum
			bestStart = runStart
		}
	}
	if bestStart < 0 {
		return Window{}, ErrDurationTooLong
	}
	return Window{
		Start:      filt[bestStart].ValidFrom,
		End:        filt[bestStart+slots-1].ValidTo,
		Slots:      append([]HalfHour(nil), filt[bestStart:bestStart+slots]...),
		MeanIncVAT: bestSum / float64(slots),
	}, nil
}

// CheapestWindowInDailyRange finds the cheapest window inside a recurring local-time
// daily range [startLocal, endLocal] anchored to `date` (the date on which the window
// starts, interpreted in tz). Only the hour/minute fields of startLocal and endLocal
// are used. If endLocal <= startLocal the range is treated as crossing midnight
// (overnight charging).
func CheapestWindowInDailyRange(
	rates []HalfHour, duration time.Duration,
	date time.Time, startLocal, endLocal time.Time, tz *time.Location,
) (Window, error) {
	if tz == nil {
		tz = time.UTC
	}
	y, m, d := date.In(tz).Date()
	from := time.Date(y, m, d, startLocal.Hour(), startLocal.Minute(), 0, 0, tz)
	to := time.Date(y, m, d, endLocal.Hour(), endLocal.Minute(), 0, 0, tz)
	if !to.After(from) {
		to = to.AddDate(0, 0, 1)
	}
	return CheapestWindow(rates, duration, from.UTC(), to.UTC())
}

// NextBelowThreshold returns the first half-hour at or after `now` whose inc-VAT rate
// is strictly less than `threshold`.
func NextBelowThreshold(rates []HalfHour, threshold float64, now time.Time) (HalfHour, error) {
	s := append([]HalfHour(nil), rates...)
	sort.Slice(s, func(i, j int) bool { return s[i].ValidFrom.Before(s[j].ValidFrom) })
	for _, r := range s {
		if r.ValidTo.After(now) && r.UnitRateIncVAT < threshold {
			return r, nil
		}
	}
	return HalfHour{}, ErrNoRates
}

// TariffCode composes an Octopus standard-unit tariff code for a given product code
// and region letter. Pattern: E-1R-<PRODUCT>-<REGION>.
func TariffCode(productCode, region string) string {
	return "E-1R-" + productCode + "-" + region
}
