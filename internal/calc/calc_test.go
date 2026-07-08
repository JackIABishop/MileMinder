package calc

import (
	"errors"
	"math"
	"reflect"
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
		Plan: &model.Plan{
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
	if !s.HasPlan {
		t.Error("HasPlan = false, want true")
	}
	if s.DailyRate != 0 {
		t.Errorf("DailyRate = %v, want 0 for a brand-new plan", s.DailyRate)
	}
	for _, f := range []float64{
		s.Delta, s.DailyRate, s.AvgAnnualMileage, s.RecentAnnualMileage, s.ProjectedEnd,
		s.EstimatedFinalMileage, s.DrivableDailyRate, s.ProjectedExcessMiles, s.ProjectedOverageCostMinor, s.PaceTrendDelta,
	} {
		if math.IsNaN(f) || math.IsInf(f, 0) {
			t.Errorf("non-finite value in new-plan status: %v", f)
		}
	}
}

func TestComputeStatus_PlainVehicle(t *testing.T) {
	now := date("2025-04-11")
	v := &model.VehicleData{
		Vehicle: "Owned Car",
		Readings: map[string]int{
			"2025-01-01": 10000,
			"2025-04-11": 12500,
		},
	}
	s := computeStatus("owned", v, now)

	if s.HasPlan {
		t.Error("HasPlan = true, want false")
	}
	if s.LatestReading != 12500 || s.LatestDate != "2025-04-11" {
		t.Errorf("latest = %d/%s, want 12500/2025-04-11", s.LatestReading, s.LatestDate)
	}
	wantDaily := 2500.0 / 100.0
	if !almostEqual(s.DailyRate, wantDaily) {
		t.Errorf("DailyRate = %v, want %v", s.DailyRate, wantDaily)
	}
	wantAnnual := wantDaily * 365.0
	if !almostEqual(s.AvgAnnualMileage, wantAnnual) {
		t.Errorf("AvgAnnualMileage = %v, want %v", s.AvgAnnualMileage, wantAnnual)
	}
	for _, f := range []float64{
		s.TargetToday, s.Delta, s.PercentUsed, s.MilesLeftYear, s.MilesLeftTerm,
		s.ProjectedEnd, s.EstimatedFinalMileage, s.DrivableDailyRate, s.ProjectedExcessMiles, s.ProjectedOverageCostMinor,
	} {
		if f != 0 {
			t.Errorf("plain allowance field not zero: %v", f)
		}
	}
}

func TestComputeStatus_PlainEdgeCases(t *testing.T) {
	now := date("2025-04-11")
	cases := []struct {
		name string
		rdgs map[string]int
	}{
		{"zero readings", nil},
		{"single reading", map[string]int{"2025-04-11": 12500}},
		{"same-day tracking", map[string]int{"2025-04-11": 12500}},
	}
	for _, tc := range cases {
		v := &model.VehicleData{Vehicle: "Owned Car", Readings: tc.rdgs}
		s := computeStatus(tc.name, v, now)
		if s.HasPlan {
			t.Errorf("%s: HasPlan = true, want false", tc.name)
		}
		for _, f := range []float64{s.DailyRate, s.AvgAnnualMileage, s.RecentAnnualMileage, s.PaceTrendDelta} {
			if f != 0 || math.IsNaN(f) || math.IsInf(f, 0) {
				t.Errorf("%s: want finite zero pace, got %v", tc.name, f)
			}
		}
	}
}

func TestEvaluateBreach(t *testing.T) {
	tests := []struct {
		name      string
		status    Status
		threshold float64
		want      Breach
	}{
		{
			name:      "plain vehicle never breaches",
			status:    Status{HasPlan: false, Delta: 1, PercentUsed: 150, ProjectedOver: true},
			threshold: 100,
			want:      Breach{},
		},
		{
			name:      "ok below threshold",
			status:    Status{HasPlan: true, PercentUsed: 51},
			threshold: 100,
			want:      Breach{},
		},
		{
			name:      "positive delta",
			status:    Status{HasPlan: true, Delta: 1, PercentUsed: 90},
			threshold: 100,
			want:      Breach{Over: true},
		},
		{
			name:      "threshold boundary",
			status:    Status{HasPlan: true, PercentUsed: 100},
			threshold: 100,
			want:      Breach{ThresholdHit: true},
		},
		{
			name:      "custom threshold",
			status:    Status{HasPlan: true, PercentUsed: 80},
			threshold: 75,
			want:      Breach{ThresholdHit: true},
		},
		{
			name:      "projected over",
			status:    Status{HasPlan: true, PercentUsed: 75, ProjectedOver: true},
			threshold: 100,
			want:      Breach{ProjectedOver: true},
		},
		{
			name:      "all reasons",
			status:    Status{HasPlan: true, Delta: 1, PercentUsed: 125, ProjectedOver: true},
			threshold: 100,
			want:      Breach{Over: true, ThresholdHit: true, ProjectedOver: true},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := EvaluateBreach(tt.status, tt.threshold)
			if got != tt.want {
				t.Fatalf("EvaluateBreach() = %+v, want %+v", got, tt.want)
			}
			if got.Breached() != (tt.want.Over || tt.want.ThresholdHit || tt.want.ProjectedOver) {
				t.Fatalf("Breached() mismatch for %+v", got)
			}
		})
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

	plain := computeStatus("plain", &model.VehicleData{
		Vehicle:  "Owned Car",
		Readings: map[string]int{"2025-01-01": 10000, "2025-04-11": 12000},
	}, now)
	fleet := []Status{under, bigMiles, worst, plain}
	got := ComputeFleetInsights(fleet)

	if got.TotalVehicles != 4 || got.PolicyVehicles != 3 || got.PlainVehicles != 1 {
		t.Errorf("vehicle counts = total %d policy %d plain %d, want 4/3/1", got.TotalVehicles, got.PolicyVehicles, got.PlainVehicles)
	}
	if got.CountOver != 2 || got.CountUnder != 1 {
		t.Errorf("CountOver/CountUnder = %d/%d, want 2/1", got.CountOver, got.CountUnder)
	}
	wantNet := under.Delta + bigMiles.Delta + worst.Delta
	if !almostEqual(got.NetDelta, wantNet) {
		t.Errorf("NetDelta = %v, want %v", got.NetDelta, wantNet)
	}
	wantTotalAnnual := under.AvgAnnualMileage + bigMiles.AvgAnnualMileage + worst.AvgAnnualMileage + plain.AvgAnnualMileage
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

func TestComputeFleetInsights_AllPlain(t *testing.T) {
	now := date("2025-04-11")
	plain := computeStatus("plain", &model.VehicleData{
		Vehicle:  "Owned Car",
		Readings: map[string]int{"2025-01-01": 10000, "2025-04-11": 12000},
	}, now)

	got := ComputeFleetInsights([]Status{plain})
	if got.TotalVehicles != 1 || got.PolicyVehicles != 0 || got.PlainVehicles != 1 {
		t.Errorf("vehicle counts = total %d policy %d plain %d, want 1/0/1", got.TotalVehicles, got.PolicyVehicles, got.PlainVehicles)
	}
	if got.WorstOffenderID != "" {
		t.Errorf("WorstOffenderID = %q, want empty for all-plain fleet", got.WorstOffenderID)
	}
	if got.CountOver != 0 || got.CountUnder != 0 || got.NetDelta != 0 || got.AvgPercentUsed != 0 {
		t.Errorf("allowance aggregates should be zero for all-plain fleet, got %+v", got)
	}
	if !almostEqual(got.TotalAvgAnnualMileage, plain.AvgAnnualMileage) {
		t.Errorf("TotalAvgAnnualMileage = %v, want %v", got.TotalAvgAnnualMileage, plain.AvgAnnualMileage)
	}
}

// TestComputeScenario_BaselineSemantics locks in issue #9 decision 3: the
// what-if is additive to the vehicle's existing trajectory, not a naive
// latest+extra. The hypothetical odometer must be (projected baseline at
// by_date) + extra_miles, and the status must be computed as of by_date.
func TestComputeScenario_BaselineSemantics(t *testing.T) {
	// 10000 mi/yr plan, 100 days in at 3000 mi → year pace 30 mi/day.
	v := vehicle("2025-01-01", "2028-01-01", 10000, 0, map[string]int{
		"2025-01-01": 0,
		"2025-04-11": 3000,
	})
	now := date("2025-04-11")
	byDate := date("2025-05-11") // 30 days out
	const extra = 600.0

	sc, err := computeScenario("test", v, extra, byDate, now)
	if err != nil {
		t.Fatalf("computeScenario returned error: %v", err)
	}

	// Baseline = latest + current daily_rate * days to by_date.
	cur := computeStatus("test", v, now)
	wantBaseline := 3000.0 + cur.DailyRate*30.0
	if !almostEqual(sc.BaselineMiles, wantBaseline) {
		t.Errorf("BaselineMiles = %v, want %v", sc.BaselineMiles, wantBaseline)
	}
	if !almostEqual(sc.HypotheticalMiles, wantBaseline+extra) {
		t.Errorf("HypotheticalMiles = %v, want %v", sc.HypotheticalMiles, wantBaseline+extra)
	}

	// The decisive assertion: because normal driving keeps happening in the gap,
	// the hypothetical odometer is strictly higher than a naive latest+extra.
	if !(sc.HypotheticalMiles > 3000.0+extra) {
		t.Errorf("HypotheticalMiles = %v, want > naive %v (baseline projection missing?)", sc.HypotheticalMiles, 3000.0+extra)
	}

	// Status is a snapshot as of by_date: latest reading is the synthetic one,
	// and delta measures the hypothetical odometer against the allowance line
	// *at by_date*, not today.
	if sc.Status.LatestDate != "2025-05-11" {
		t.Errorf("Status.LatestDate = %q, want 2025-05-11", sc.Status.LatestDate)
	}
	roundedHyp := math.Round(sc.HypotheticalMiles)
	if float64(sc.Status.LatestReading) != roundedHyp {
		t.Errorf("Status.LatestReading = %d, want %v", sc.Status.LatestReading, roundedHyp)
	}
	wantDelta := roundedHyp - AllowanceMiles(10000, date("2025-01-01"), byDate)
	if !almostEqual(sc.Status.Delta, wantDelta) {
		t.Errorf("Status.Delta = %v, want %v (delta must be measured as of by_date)", sc.Status.Delta, wantDelta)
	}
	if sc.ByDate != "2025-05-11" || sc.ExtraMiles != extra {
		t.Errorf("echoed inputs = %q/%v, want 2025-05-11/%v", sc.ByDate, sc.ExtraMiles, extra)
	}
}

// TestComputeScenario_ZeroExtraIsPureBaseline: extra_miles = 0 answers the pure
// "where will I be by X at my current pace" question.
func TestComputeScenario_ZeroExtraIsPureBaseline(t *testing.T) {
	v := vehicle("2025-01-01", "2028-01-01", 10000, 0, map[string]int{
		"2025-01-01": 0,
		"2025-04-11": 3000,
	})
	sc, err := computeScenario("test", v, 0, date("2025-05-11"), date("2025-04-11"))
	if err != nil {
		t.Fatalf("computeScenario returned error: %v", err)
	}
	if !almostEqual(sc.HypotheticalMiles, sc.BaselineMiles) {
		t.Errorf("with zero extra: HypotheticalMiles %v != BaselineMiles %v", sc.HypotheticalMiles, sc.BaselineMiles)
	}
}

// TestComputeScenario_CrossesYearBoundary: when by_date lands in the *next*
// allowance year, the status segment must move forward to that year — proving
// the status is computed as of by_date, not as of today.
func TestComputeScenario_CrossesYearBoundary(t *testing.T) {
	// Plan year boundaries fall on 06-01. Today sits in year 1; by_date in year 2.
	v := vehicle("2024-06-01", "2027-06-01", 10000, 0, map[string]int{
		"2024-06-01": 0,
		"2025-04-01": 3000,
	})
	now := date("2025-04-01")    // segment 2024-06-01 → 2025-06-01
	byDate := date("2025-07-01") // segment 2025-06-01 → 2026-06-01

	sc, err := computeScenario("test", v, 500, byDate, now)
	if err != nil {
		t.Fatalf("computeScenario returned error: %v", err)
	}
	if sc.Status.LatestDate != "2025-07-01" {
		t.Errorf("Status.LatestDate = %q, want 2025-07-01", sc.Status.LatestDate)
	}
	// Days left in by_date's allowance year: 2025-07-01 → 2026-06-01 = 335 days.
	// Computed as-of-today it would be ~61 (2025-04-01 → 2025-06-01).
	segmentEnd := date("2026-06-01")
	wantDaysLeft := int(math.Ceil(segmentEnd.Sub(byDate).Hours() / 24.0))
	if sc.Status.DaysLeftYear != wantDaysLeft {
		t.Errorf("Status.DaysLeftYear = %d, want %d (segment should follow by_date, not today)", sc.Status.DaysLeftYear, wantDaysLeft)
	}
}

func TestComputeScenario_Errors(t *testing.T) {
	plan := func(rdgs map[string]int) *model.VehicleData {
		return vehicle("2025-01-01", "2028-01-01", 10000, 0, rdgs)
	}
	valid := map[string]int{"2025-01-01": 0, "2025-04-11": 3000}

	cases := []struct {
		name    string
		data    *model.VehicleData
		extra   float64
		byDate  time.Time
		now     time.Time
		wantErr error
	}{
		{"no plan", &model.VehicleData{Vehicle: "Owned", Readings: valid}, 100, date("2025-05-11"), date("2025-04-11"), ErrScenarioNoPlan},
		{"negative extra", plan(valid), -1, date("2025-05-11"), date("2025-04-11"), ErrScenarioNegativeMiles},
		{"no readings", plan(map[string]int{}), 100, date("2025-05-11"), date("2025-04-11"), ErrScenarioNoReadings},
		{"by_date is today", plan(valid), 100, date("2025-04-11"), date("2025-04-11"), ErrScenarioDateNotFuture},
		{"by_date in past", plan(valid), 100, date("2025-03-01"), date("2025-04-11"), ErrScenarioDateNotFuture},
		{"by_date == latest reading", plan(valid), 100, date("2025-04-11"), date("2025-04-01"), ErrScenarioDateNotFuture},
		{"by_date after plan end", plan(valid), 100, date("2028-06-01"), date("2025-04-11"), ErrScenarioAfterPlanEnd},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := computeScenario("test", tc.data, tc.extra, tc.byDate, tc.now)
			if !errors.Is(err, tc.wantErr) {
				t.Errorf("computeScenario err = %v, want %v", err, tc.wantErr)
			}
		})
	}
}

// TestComputeScenario_DoesNotMutateInput is the calc-level half of issue #9's
// core safety property: the projection is read-only. The API test asserts the
// same at the storage layer.
func TestComputeScenario_DoesNotMutateInput(t *testing.T) {
	rdgs := map[string]int{"2025-01-01": 0, "2025-04-11": 3000}
	v := vehicle("2025-01-01", "2028-01-01", 10000, 0, rdgs)

	before := make(map[string]int, len(v.Readings))
	for k, val := range v.Readings {
		before[k] = val
	}

	if _, err := computeScenario("test", v, 600, date("2025-05-11"), date("2025-04-11")); err != nil {
		t.Fatalf("computeScenario returned error: %v", err)
	}

	if !reflect.DeepEqual(v.Readings, before) {
		t.Errorf("computeScenario mutated input readings: got %v, want %v", v.Readings, before)
	}
	if _, ok := v.Readings["2025-05-11"]; ok {
		t.Error("computeScenario added the synthetic reading to the caller's map")
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
	if unset.ProjectedOverageCostMinor != 0 {
		t.Errorf("ProjectedOverageCostMinor (unset) = %v, want 0", unset.ProjectedOverageCostMinor)
	}

	set := computeStatus("set", mk(10), now) // 10 minor units/excess mile
	wantCost := wantExcess * 10.0            // cost stays in minor units; clients convert
	if !almostEqual(set.ProjectedOverageCostMinor, wantCost) {
		t.Errorf("ProjectedOverageCostMinor (set) = %v, want %v", set.ProjectedOverageCostMinor, wantCost)
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
