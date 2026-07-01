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
	for _, f := range []float64{
		s.Delta, s.DailyRate, s.AvgAnnualMileage, s.RecentAnnualMileage, s.ProjectedEnd,
		s.EstimatedFinalMileage, s.DrivableDailyRate, s.ProjectedExcessMiles, s.ProjectedOverageCost, s.PaceTrendDelta,
	} {
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

func TestComputeFleetInsights(t *testing.T) {
	now := date("2025-04-11") // 100 days after a 2025-01-01 start

	// Build three vehicles via the real computeStatus core so the roll-up is
	// tested against genuine Status values, not hand-faked numbers.
	// allowance ≈ 2740 mi to date for a 10000 mi/yr plan.
	under := computeStatus("under", vehicle("2025-01-01", "2028-01-01", 10000, 0, map[string]int{
		"2025-01-01": 0,
		"2025-04-11": 1500, // well under the line → negative delta
	}), now)
	// Big absolute delta in miles, but a generous 30000 mi/yr allowance keeps the
	// *percentage* used modest — it must NOT be picked as worst offender.
	bigMiles := computeStatus("bigmiles", vehicle("2025-01-01", "2028-01-01", 30000, 0, map[string]int{
		"2025-01-01": 0,
		"2025-04-11": 11000, // ~+2700 mi delta, but only ~134% of a large allowance
	}), now)
	// Small absolute delta, but a tiny 2000 mi/yr allowance makes it proportionally
	// the worst — this is the expected worst offender (ranked by percent_used).
	worst := computeStatus("worst", vehicle("2025-01-01", "2028-01-01", 2000, 0, map[string]int{
		"2025-01-01": 0,
		"2025-04-11": 1500, // ~+950 mi delta, but ~273% of a small allowance
	}), now)

	// Sanity-check the fixture intent: bigMiles has the larger absolute delta,
	// worst has the larger percentage used.
	if !(bigMiles.Delta > worst.Delta) {
		t.Fatalf("fixture broken: want bigMiles.Delta(%v) > worst.Delta(%v)", bigMiles.Delta, worst.Delta)
	}
	if !(worst.PercentUsed > bigMiles.PercentUsed) {
		t.Fatalf("fixture broken: want worst.PercentUsed(%v) > bigMiles.PercentUsed(%v)", worst.PercentUsed, bigMiles.PercentUsed)
	}

	fleet := []Status{under, bigMiles, worst}
	got := ComputeFleetInsights(fleet)

	if got.TotalVehicles != 3 {
		t.Errorf("TotalVehicles = %d, want 3", got.TotalVehicles)
	}
	if got.CountOver != 2 || got.CountUnder != 1 {
		t.Errorf("CountOver/CountUnder = %d/%d, want 2/1", got.CountOver, got.CountUnder)
	}
	wantNet := under.Delta + bigMiles.Delta + worst.Delta
	if !almostEqual(got.NetDelta, wantNet) {
		t.Errorf("NetDelta = %v, want %v", got.NetDelta, wantNet)
	}
	wantTotalAnnual := under.AvgAnnualMileage + bigMiles.AvgAnnualMileage + worst.AvgAnnualMileage
	if !almostEqual(got.TotalAvgAnnualMileage, wantTotalAnnual) {
		t.Errorf("TotalAvgAnnualMileage = %v, want %v", got.TotalAvgAnnualMileage, wantTotalAnnual)
	}
	wantAvgPct := (under.PercentUsed + bigMiles.PercentUsed + worst.PercentUsed) / 3.0
	if !almostEqual(got.AvgPercentUsed, wantAvgPct) {
		t.Errorf("AvgPercentUsed = %v, want %v", got.AvgPercentUsed, wantAvgPct)
	}
	// Ranked by percent_used, not absolute miles.
	if got.WorstOffenderID != "worst" {
		t.Errorf("WorstOffenderID = %q, want \"worst\" (ranking should be by percent_used, not miles)", got.WorstOffenderID)
	}
	if got.WorstOffenderVehicle != worst.Vehicle {
		t.Errorf("WorstOffenderVehicle = %q, want %q", got.WorstOffenderVehicle, worst.Vehicle)
	}
}

func TestComputeFleetInsights_SingleCar(t *testing.T) {
	now := date("2025-04-11")
	only := computeStatus("solo", vehicle("2025-01-01", "2028-01-01", 10000, 0, map[string]int{
		"2025-01-01": 0,
		"2025-04-11": 5000,
	}), now)

	got := ComputeFleetInsights([]Status{only})
	if got.TotalVehicles != 1 {
		t.Errorf("TotalVehicles = %d, want 1", got.TotalVehicles)
	}
	if got.WorstOffenderID != "solo" {
		t.Errorf("WorstOffenderID = %q, want \"solo\" (a single car is trivially the worst)", got.WorstOffenderID)
	}
	if !almostEqual(got.AvgPercentUsed, only.PercentUsed) {
		t.Errorf("AvgPercentUsed = %v, want %v", got.AvgPercentUsed, only.PercentUsed)
	}
}

func TestComputeFleetInsights_Empty(t *testing.T) {
	got := ComputeFleetInsights(nil)
	if got.TotalVehicles != 0 {
		t.Errorf("TotalVehicles = %d, want 0", got.TotalVehicles)
	}
	if got.WorstOffenderID != "" {
		t.Errorf("WorstOffenderID = %q, want empty", got.WorstOffenderID)
	}
	if got.NetDelta != 0 || got.TotalAvgAnnualMileage != 0 || got.AvgPercentUsed != 0 {
		t.Errorf("empty fleet should have zero sums, got %+v", got)
	}
}

// TestRenewalCountdown locks #3: days_to_end is the whole countdown to plan end,
// and estimated_final_mileage projects from the latest reading at the current
// daily pace (daily_rate).
func TestRenewalCountdown(t *testing.T) {
	now := date("2025-04-11") // 100 days into a one-year plan
	v := vehicle("2025-01-01", "2026-01-01", 3650, 0, map[string]int{
		"2025-01-01": 0,
		"2025-04-11": 1000, // 1000 mi over 100 days → daily_rate = 10
	})
	s := computeStatus("test", v, now)

	termDays := date("2026-01-01").Sub(now).Hours() / 24.0
	if want := int(math.Ceil(termDays)); s.DaysToEnd != want {
		t.Errorf("DaysToEnd = %d, want %d", s.DaysToEnd, want)
	}
	if !almostEqual(s.DailyRate, 10) {
		t.Fatalf("precondition: DailyRate = %v, want 10", s.DailyRate)
	}
	wantFinal := 1000.0 + 10.0*termDays
	if !almostEqual(s.EstimatedFinalMileage, wantFinal) {
		t.Errorf("EstimatedFinalMileage = %v, want %v", s.EstimatedFinalMileage, wantFinal)
	}
}

// TestExpiredPlan locks the past-plan-end behaviour of the renewal/projection
// figures: termDays is clamped at 0, so days_to_end never goes negative and the
// final-mileage estimate never projects backward from the latest reading.
func TestExpiredPlan(t *testing.T) {
	now := date("2025-06-01") // five months after the plan ended
	v := vehicle("2024-01-01", "2025-01-01", 10000, 0, map[string]int{
		"2024-01-01": 0,
		"2024-12-01": 12000,
	})
	s := computeStatus("expired", v, now)

	if s.DaysToEnd != 0 {
		t.Errorf("DaysToEnd = %d, want 0 once the plan has ended", s.DaysToEnd)
	}
	if !almostEqual(s.EstimatedFinalMileage, 12000) {
		t.Errorf("EstimatedFinalMileage = %v, want the latest reading (12000), not a backward projection", s.EstimatedFinalMileage)
	}
	if s.DrivableDailyRate != 0 {
		t.Errorf("DrivableDailyRate = %v, want 0 with no days left", s.DrivableDailyRate)
	}
}

// TestDrivableDailyRate locks #4: the safe mi/day for the rest of the plan is a
// capacity figure (remaining allowed miles ÷ remaining days), clamped to 0 once
// you've already used the whole term allowance.
func TestDrivableDailyRate(t *testing.T) {
	now := date("2025-04-11")
	termDays := date("2026-01-01").Sub(now).Hours() / 24.0

	// Under budget: 3650 mi total allowance over the year, 1000 used so far.
	under := vehicle("2025-01-01", "2026-01-01", 3650, 0, map[string]int{
		"2025-01-01": 0,
		"2025-04-11": 1000,
	})
	su := computeStatus("under", under, now)
	wantRate := (3650.0 - 1000.0) / termDays
	if !almostEqual(su.DrivableDailyRate, wantRate) {
		t.Errorf("under-budget DrivableDailyRate = %v, want %v", su.DrivableDailyRate, wantRate)
	}

	// Over budget: only 500 mi allowed for the whole term but 1000 already used.
	over := vehicle("2025-01-01", "2026-01-01", 500, 0, map[string]int{
		"2025-01-01": 0,
		"2025-04-11": 1000,
	})
	so := computeStatus("over", over, now)
	if so.DrivableDailyRate != 0 {
		t.Errorf("over-budget DrivableDailyRate = %v, want 0 (clamped)", so.DrivableDailyRate)
	}
}

// TestOverageCost locks #5: projected excess miles are computed regardless of an
// excess rate, but the £ penalty is only populated when a rate is set.
func TestOverageCost(t *testing.T) {
	now := date("2025-04-11")
	termDays := date("2026-01-01").Sub(now).Hours() / 24.0
	// daily_rate = 10; estimated final = 1000 + 10*termDays; allowance over the
	// full year = 1000 mi, so the plan is projected well over.
	mk := func(rate int) *model.VehicleData {
		v := vehicle("2025-01-01", "2026-01-01", 1000, 0, map[string]int{
			"2025-01-01": 0,
			"2025-04-11": 1000,
		})
		v.Plan.ExcessRate = rate
		return v
	}

	wantExcess := (1000.0 + 10.0*termDays) - 1000.0 // projected miles driven − total allowance

	unset := computeStatus("unset", mk(0), now)
	if !almostEqual(unset.ProjectedExcessMiles, wantExcess) {
		t.Errorf("ProjectedExcessMiles (unset) = %v, want %v", unset.ProjectedExcessMiles, wantExcess)
	}
	if unset.ProjectedOverageCost != 0 {
		t.Errorf("ProjectedOverageCost (unset) = %v, want 0", unset.ProjectedOverageCost)
	}

	set := computeStatus("set", mk(10), now) // 10 pence/excess mile
	wantCost := wantExcess * 10.0 / 100.0
	if !almostEqual(set.ProjectedOverageCost, wantCost) {
		t.Errorf("ProjectedOverageCost (set) = %v, want %v", set.ProjectedOverageCost, wantCost)
	}
}

// TestPaceTrend locks #7: recent 90-day pace vs lifetime average is classified
// as accelerating / easing / steady against a ±5% threshold.
func TestPaceTrend(t *testing.T) {
	now := date("2025-04-01")
	cases := []struct {
		name string
		rdgs map[string]int
		want string
	}{
		{"accelerating", map[string]int{ // light first year, heavy recent
			"2024-01-01": 0, "2025-01-01": 5000, "2025-04-01": 8000,
		}, "accelerating"},
		{"easing", map[string]int{ // heavy first year, light recent
			"2024-01-01": 0, "2025-01-01": 15000, "2025-04-01": 16000,
		}, "easing"},
		{"steady", map[string]int{ // constant pace → recent ≈ lifetime
			"2024-01-01": 0, "2025-04-01": 4560,
		}, "steady"},
	}
	for _, tc := range cases {
		v := vehicle("2024-01-01", "2027-01-01", 10000, 0, tc.rdgs)
		s := computeStatus(tc.name, v, now)
		if s.PaceTrend != tc.want {
			t.Errorf("%s: PaceTrend = %q (delta=%v, recent=%v, avg=%v), want %q",
				tc.name, s.PaceTrend, s.PaceTrendDelta, s.RecentAnnualMileage, s.AvgAnnualMileage, tc.want)
		}
	}
}
