package calc

import (
	"math"
	"testing"
	"time"

	"github.com/jackiabishop/mileminder/internal/model"
)

func date(s string) time.Time {
	t, err := time.Parse("2006-01-02", s)
	if err != nil {
		panic(err)
	}
	return t
}

func readings(pairs ...DatedReading) []DatedReading { return pairs }

func r(d string, m float64) DatedReading { return DatedReading{Date: date(d), Miles: m} }

const eps = 1e-6

func almostEqual(a, b float64) bool { return math.Abs(a-b) <= eps }

func TestOdometerAt(t *testing.T) {
	rs := readings(
		r("2025-01-01", 1000),
		r("2025-01-11", 1100), // +100 miles over 10 days
		r("2025-02-10", 1400),
	)

	tests := []struct {
		name string
		at   time.Time
		want float64
		ok   bool
	}{
		{"exact first", date("2025-01-01"), 1000, true},
		{"exact middle", date("2025-01-11"), 1100, true},
		{"exact last", date("2025-02-10"), 1400, true},
		{"midpoint interpolation", date("2025-01-06"), 1050, true}, // halfway in first segment
		{"clamp before first", date("2024-06-01"), 1000, true},
		{"clamp after last", date("2026-01-01"), 1400, true},
	}
	for _, tc := range tests {
		got, ok := OdometerAt(rs, tc.at)
		if ok != tc.ok || !almostEqual(got, tc.want) {
			t.Errorf("%s: OdometerAt = (%v, %v), want (%v, %v)", tc.name, got, ok, tc.want, tc.ok)
		}
	}
}

func TestOdometerAt_Empty(t *testing.T) {
	if got, ok := OdometerAt(nil, date("2025-01-01")); ok || got != 0 {
		t.Errorf("empty: got (%v, %v), want (0, false)", got, ok)
	}
}

func TestOdometerAt_SingleReading(t *testing.T) {
	rs := readings(r("2025-01-01", 1000))
	for _, at := range []time.Time{date("2024-01-01"), date("2025-01-01"), date("2026-01-01")} {
		got, ok := OdometerAt(rs, at)
		if !ok || got != 1000 {
			t.Errorf("single @ %s: got (%v, %v), want (1000, true)", at.Format("2006-01-02"), got, ok)
		}
	}
}

func TestOdometerAt_SameDateZeroSpan(t *testing.T) {
	// Two readings on the same date: interpolation must not divide by zero.
	rs := readings(r("2025-01-01", 1000), r("2025-01-01", 1200), r("2025-02-01", 1500))
	if got, ok := OdometerAt(rs, date("2025-01-15")); !ok || math.IsNaN(got) || math.IsInf(got, 0) {
		t.Errorf("same-date span: got (%v, %v), want finite value", got, ok)
	}
}

// vehicle is a small builder for test plans.
func vehicle(start, end string, allowance, startMiles int, rdgs map[string]int) *model.VehicleData {
	return &model.VehicleData{
		Vehicle: "Test Car",
		Plan: model.Plan{
			Start:           date(start),
			End:             date(end),
			AnnualAllowance: allowance,
			StartMiles:      startMiles,
		},
		Readings: rdgs,
	}
}

// TestDailyRate_YearBoundary locks the PR #2 fix: miles driven *before* the
// current allowance year must not leak into daily_rate. The plan started over a
// year ago; the car drove heavily in year 1 and lightly in year 2. daily_rate
// must reflect only the year-2 window.
func TestDailyRate_YearBoundary(t *testing.T) {
	v := vehicle("2024-01-01", "2027-01-01", 10000, 0, map[string]int{
		"2024-01-01": 0,
		"2024-12-31": 15000, // heavy first year
		"2025-04-01": 17000, // light second year: +2000 since the boundary
	})
	now := date("2025-04-01")
	s := computeStatus("test", v, now)

	// Segment boundary is 2025-01-01; odometer there ≈ interpolated value.
	rs := SortedReadings(v)
	segMiles, _ := OdometerAt(rs, date("2025-01-01"))
	milesThisYear := 17000 - segMiles
	daysThisYear := now.Sub(date("2025-01-01")).Hours() / 24.0
	wantRate := milesThisYear / daysThisYear

	if !almostEqual(s.DailyRate, wantRate) {
		t.Errorf("DailyRate = %v, want %v (pre-year driving leaked in?)", s.DailyRate, wantRate)
	}
	// Sanity: the year-2 pace is far below the year-1 pace.
	if s.DailyRate > 30 {
		t.Errorf("DailyRate %v looks like it includes year-1 driving", s.DailyRate)
	}
}

func TestComputeStatus_UnderOverOnPace(t *testing.T) {
	// Plan: 10000 mi/yr from 2025-01-01, 100 days elapsed → allowance ≈ 2740 mi.
	now := date("2025-04-11") // 100 days after start
	allowance := 10000.0
	days := now.Sub(date("2025-01-01")).Hours() / 24.0
	expectedAllowance := allowance * days / 365.0

	cases := []struct {
		name     string
		latest   int
		wantOver bool // delta > 0 means over budget
	}{
		{"under pace", int(expectedAllowance) - 1000, false},
		{"over pace", int(expectedAllowance) + 1000, true},
	}
	for _, tc := range cases {
		v := vehicle("2025-01-01", "2028-01-01", 10000, 0, map[string]int{
			"2025-01-01": 0,
			"2025-04-11": tc.latest,
		})
		s := computeStatus("test", v, now)
		over := s.Delta > 0
		if over != tc.wantOver {
			t.Errorf("%s: Delta=%v over=%v, want over=%v", tc.name, s.Delta, over, tc.wantOver)
		}
	}

	// On pace: latest exactly on the allowance line → delta ≈ 0.
	v := vehicle("2025-01-01", "2028-01-01", 10000, 0, map[string]int{
		"2025-01-01": 0,
		"2025-04-11": int(math.Round(expectedAllowance)),
	})
	s := computeStatus("test", v, now)
	if math.Abs(s.Delta) > 1.0 {
		t.Errorf("on pace: Delta = %v, want ~0", s.Delta)
	}
}

// TestDelta_ExcludesStartMiles locks CLI bug-fix #1: delta and percent_used
// must be independent of the absolute start_miles (they measure miles driven
// against the allowance line, not raw odometer vs target).
func TestDelta_ExcludesStartMiles(t *testing.T) {
	now := date("2025-04-11")
	low := vehicle("2025-01-01", "2028-01-01", 10000, 0, map[string]int{
		"2025-01-01": 0,
		"2025-04-11": 3000,
	})
	high := vehicle("2025-01-01", "2028-01-01", 10000, 50000, map[string]int{
		"2025-01-01": 50000,
		"2025-04-11": 53000, // same 3000 miles driven
	})
	ls := computeStatus("low", low, now)
	hs := computeStatus("high", high, now)
	if !almostEqual(ls.Delta, hs.Delta) {
		t.Errorf("Delta leaks start_miles: low=%v high=%v", ls.Delta, hs.Delta)
	}
	if !almostEqual(ls.PercentUsed, hs.PercentUsed) {
		t.Errorf("PercentUsed leaks start_miles: low=%v high=%v", ls.PercentUsed, hs.PercentUsed)
	}
}

func TestComputeStatus_NewPlan(t *testing.T) {
	// Plan started a few days ago with a single baseline reading.
	now := date("2025-01-05")
	v := vehicle("2025-01-01", "2028-01-01", 10000, 12000, map[string]int{
		"2025-01-01": 12000,
	})
	s := computeStatus("new", v, now)

	if s.LatestReading != 12000 {
		t.Errorf("LatestReading = %d, want 12000", s.LatestReading)
	}
	if s.DailyRate != 0 {
		t.Errorf("DailyRate = %v, want 0 for a brand-new plan", s.DailyRate)
	}
	for _, f := range []float64{s.Delta, s.DailyRate, s.AvgAnnualMileage, s.RecentAnnualMileage, s.ProjectedEnd} {
		if math.IsNaN(f) || math.IsInf(f, 0) {
			t.Errorf("non-finite value in new-plan status: %v", f)
		}
	}
}

func TestAllowanceMiles(t *testing.T) {
	start := date("2025-01-01")
	if got := AllowanceMiles(10000, start, start); got != 0 {
		t.Errorf("at start: got %v, want 0", got)
	}
	if got := AllowanceMiles(10000, start, date("2024-06-01")); got != 0 {
		t.Errorf("before start: got %v, want 0 (clamped)", got)
	}
	want := 10000.0 * 365.0 / 365.0
	if got := AllowanceMiles(10000, start, date("2026-01-01")); !almostEqual(got, want) {
		t.Errorf("one year: got %v, want %v", got, want)
	}
}
