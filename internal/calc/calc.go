// Package calc is the single source of truth for MileMinder's mileage,
// status, projection and pace calculations. Both internal/api and cmd call
// into it so the web dashboard, the CLI and (eventually) the mobile client all
// compute identical numbers.
package calc

import (
	"math"
	"sort"
	"time"

	"github.com/jackiabishop/mileminder/internal/model"
)

// Status represents computed status for a vehicle. JSON tags mirror the
// web/iOS API contract — keep them and the field order stable.
type Status struct {
	ID                  string    `json:"id"`
	Vehicle             string    `json:"vehicle"`
	LatestReading       int       `json:"latest_reading"`
	LatestDate          string    `json:"latest_date"`
	TargetToday         float64   `json:"target_today"`
	Delta               float64   `json:"delta"`
	PercentUsed         float64   `json:"percent_used"`
	DaysLeftYear        int       `json:"days_left_year"`
	MilesLeftYear       float64   `json:"miles_left_year"`
	DaysLeftTerm        int       `json:"days_left_term"`
	MilesLeftTerm       float64   `json:"miles_left_term"`
	YearsLeftTerm       int       `json:"years_left_term"`
	DailyRate           float64   `json:"daily_rate"`
	AvgAnnualMileage    float64   `json:"avg_annual_mileage"`
	RecentAnnualMileage float64   `json:"recent_annual_mileage"`
	ProjectedEnd        float64   `json:"projected_end"`
	ProjectedOver       bool      `json:"projected_over"`
	PlanStart           time.Time `json:"plan_start"`
	PlanEnd             time.Time `json:"plan_end"`
	AnnualAllowance     int       `json:"annual_allowance"`
	StartMiles          int       `json:"start_miles"`
	IsDefault           bool      `json:"is_default"`

	// Renewal countdown + final-mileage estimate (#3). DaysToEnd is the total
	// days until the plan ends — distinct from DaysLeftTerm, which is the
	// remainder after whole years. EstimatedFinalMileage projects the odometer
	// at plan end at the current daily pace.
	DaysToEnd             int     `json:"days_to_end"`
	EstimatedFinalMileage float64 `json:"estimated_final_mileage"`

	// Drivable-rate budget (#4): the safe miles/day you can drive for the rest
	// of the plan and still finish within the total term allowance. A capacity
	// figure (remaining allowed miles ÷ remaining days), not a pace projection.
	DrivableDailyRate float64 `json:"drivable_daily_rate"`

	// Overage cost estimate (#5). ExcessRate mirrors plan.ExcessRate (pence per
	// excess mile). ProjectedExcessMiles is how far the projected final mileage
	// exceeds the total term allowance; ProjectedOverageCost is the £ penalty,
	// populated only when an excess rate is set.
	ExcessRate           int     `json:"excess_rate,omitempty"`
	ProjectedExcessMiles float64 `json:"projected_excess_miles"`
	ProjectedOverageCost float64 `json:"projected_overage_cost,omitempty"`

	// Trend signal (#7): recent (90-day) annual pace vs lifetime average.
	// PaceTrend classifies the delta as accelerating / easing / steady.
	PaceTrendDelta float64 `json:"pace_trend_delta"`
	PaceTrend      string  `json:"pace_trend"`
}

// DatedReading is a single odometer reading with a parsed date.
type DatedReading struct {
	Date  time.Time
	Miles float64
}

// SortedReadings returns the vehicle's readings parsed and sorted by date.
func SortedReadings(data *model.VehicleData) []DatedReading {
	out := make([]DatedReading, 0, len(data.Readings))
	for ds, m := range data.Readings {
		if t, err := time.Parse("2006-01-02", ds); err == nil {
			out = append(out, DatedReading{Date: t, Miles: float64(m)})
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Date.Before(out[j].Date) })
	return out
}

// OdometerAt estimates the odometer reading at time `at` by linearly
// interpolating between the two readings that bracket it. Outside the data
// range it clamps to the first/last reading. Returns false if there are no
// readings to work from.
func OdometerAt(rs []DatedReading, at time.Time) (float64, bool) {
	if len(rs) == 0 {
		return 0, false
	}
	if !at.After(rs[0].Date) {
		return rs[0].Miles, true
	}
	if !at.Before(rs[len(rs)-1].Date) {
		return rs[len(rs)-1].Miles, true
	}
	for i := 1; i < len(rs); i++ {
		if !at.After(rs[i].Date) {
			a, b := rs[i-1], rs[i]
			span := b.Date.Sub(a.Date).Seconds()
			if span <= 0 {
				return b.Miles, true
			}
			frac := at.Sub(a.Date).Seconds() / span
			return a.Miles + (b.Miles-a.Miles)*frac, true
		}
	}
	return rs[len(rs)-1].Miles, true
}

// AllowanceMiles returns the ideal/allowance-line mileage at time `at`: a
// straight line of annualAllowance miles per 365 days from the plan start,
// clamped to zero before the plan begins. This is the shared primitive behind
// both the status target and the graph's ideal series.
func AllowanceMiles(annualAllowance int, planStart, at time.Time) float64 {
	daysElapsed := at.Sub(planStart).Hours() / 24.0
	if daysElapsed < 0 {
		daysElapsed = 0
	}
	return float64(annualAllowance) * daysElapsed / 365.0
}

// ComputeStatus calculates all status metrics for a vehicle as of now.
func ComputeStatus(id string, data *model.VehicleData) Status {
	return computeStatus(id, data, time.Now())
}

// computeStatus is the deterministic core. `now` is injected so the math is
// testable against a fixed clock.
func computeStatus(id string, data *model.VehicleData, now time.Time) Status {
	today := now

	// Find latest reading
	var dates []string
	for d := range data.Readings {
		dates = append(dates, d)
	}
	sort.Strings(dates)

	latestDate := ""
	latestMiles := data.Plan.StartMiles
	if len(dates) > 0 {
		latestDate = dates[len(dates)-1]
		latestMiles = data.Readings[latestDate]
	}

	// Compute target vs actual
	daysElapsed := today.Sub(data.Plan.Start).Hours() / 24.0
	if daysElapsed < 0 {
		daysElapsed = 0
	}
	targetToday := float64(data.Plan.StartMiles) + float64(data.Plan.AnnualAllowance)*daysElapsed/365.0
	milesUsed := float64(latestMiles - data.Plan.StartMiles)
	delta := milesUsed - (targetToday - float64(data.Plan.StartMiles))

	var pctUsed float64
	targetMileage := targetToday - float64(data.Plan.StartMiles)
	if targetMileage > 0 {
		pctUsed = milesUsed / targetMileage * 100.0
	}

	// Year left calculation
	yearsSince := today.Year() - data.Plan.Start.Year()
	segmentStart := data.Plan.Start.AddDate(yearsSince, 0, 0)
	if segmentStart.After(today) {
		segmentStart = segmentStart.AddDate(-1, 0, 0)
	}
	segmentEnd := segmentStart.AddDate(1, 0, 0)
	if segmentEnd.After(data.Plan.End) {
		segmentEnd = data.Plan.End
	}

	daysLeftYear := segmentEnd.Sub(today).Hours() / 24.0
	if daysLeftYear < 0 {
		daysLeftYear = 0
	}
	milesLeftYear := float64(data.Plan.AnnualAllowance) * daysLeftYear / 365.0

	// Daily rate within the current allowance year. Interpolate the odometer
	// exactly at the segment boundary so miles driven *before* this year started
	// aren't counted against it — both numerator and denominator then cover the
	// same window (segmentStart → today).
	readings := SortedReadings(data)
	segmentDurationDays := segmentEnd.Sub(segmentStart).Hours() / 24.0
	daysSoFar := today.Sub(segmentStart).Hours() / 24.0
	if daysSoFar < 1 {
		daysSoFar = 1
	}
	milesSoFar := 0.0
	if segMiles, ok := OdometerAt(readings, segmentStart); ok {
		milesSoFar = float64(latestMiles) - segMiles
		if milesSoFar < 0 {
			milesSoFar = 0
		}
	}
	dailyRate := milesSoFar / daysSoFar

	// Realised lifetime average annual mileage: total miles driven since the
	// plan start, annualised over the elapsed period. This is the stable
	// figure to quote for insurance, unlike the recent-pace daily rate.
	avgAnnualMileage := 0.0
	if daysElapsed >= 1 {
		avgAnnualMileage = milesUsed / daysElapsed * 365.0
	}

	// Recent annual mileage: pace over the trailing 90 days, annualised. If
	// there's less than 90 days of history, measure from the first reading.
	recentAnnualMileage := 0.0
	if len(readings) > 0 {
		windowStart := today.AddDate(0, 0, -90)
		if windowStart.Before(readings[0].Date) {
			windowStart = readings[0].Date
		}
		windowDays := today.Sub(windowStart).Hours() / 24.0
		if windowDays >= 1 {
			if baseMiles, ok := OdometerAt(readings, windowStart); ok {
				recentAnnualMileage = (float64(latestMiles) - baseMiles) / windowDays * 365.0
			}
		}
	}

	projectedUsage := dailyRate * segmentDurationDays
	allowanceSegment := float64(data.Plan.AnnualAllowance) * segmentDurationDays / 365.0
	projectedEnd := allowanceSegment - projectedUsage
	projectedOver := projectedEnd < 0
	if projectedOver {
		projectedEnd = -projectedEnd
	}

	// Term left
	termDays := data.Plan.End.Sub(today).Hours() / 24.0
	if termDays < 0 {
		termDays = 0
	}
	yearsLeft := int(termDays / 365.0)
	daysLeft := int(math.Mod(termDays, 365.0))
	milesLeftTerm := float64(data.Plan.AnnualAllowance) * termDays / 365.0

	// Renewal countdown + final-mileage estimate (#3). daysToEnd is the whole
	// countdown to plan end; the final-mileage estimate continues from the
	// latest reading at the current daily pace.
	daysToEnd := int(math.Ceil(termDays))
	estimatedFinalMileage := float64(latestMiles) + dailyRate*termDays

	// Drivable-rate budget (#4): how many miles/day you can still drive for the
	// rest of the plan and finish within the total term allowance. Capacity, not
	// pace: (total term allowance − miles already used) ÷ days remaining.
	totalTermDays := data.Plan.End.Sub(data.Plan.Start).Hours() / 24.0
	totalTermAllowanceMiles := float64(data.Plan.AnnualAllowance) * totalTermDays / 365.0
	drivableDailyRate := 0.0
	if termDays >= 1 {
		drivableDailyRate = (totalTermAllowanceMiles - milesUsed) / termDays
		if drivableDailyRate < 0 {
			drivableDailyRate = 0
		}
	}

	// Overage cost estimate (#5): how far the projected final mileage overshoots
	// the total term allowance, and the £ penalty at the plan's excess rate.
	projectedMilesDriven := estimatedFinalMileage - float64(data.Plan.StartMiles)
	projectedExcessMiles := projectedMilesDriven - totalTermAllowanceMiles
	if projectedExcessMiles < 0 {
		projectedExcessMiles = 0
	}
	projectedOverageCost := 0.0
	if data.Plan.ExcessRate > 0 {
		projectedOverageCost = projectedExcessMiles * float64(data.Plan.ExcessRate) / 100.0
	}

	// Trend signal (#7): recent 90-day annual pace vs the lifetime average.
	paceTrendDelta := recentAnnualMileage - avgAnnualMileage
	paceTrend := "steady"
	if avgAnnualMileage > 0 {
		threshold := avgAnnualMileage * 0.05
		if paceTrendDelta > threshold {
			paceTrend = "accelerating"
		} else if paceTrendDelta < -threshold {
			paceTrend = "easing"
		}
	}

	return Status{
		ID:                  id,
		Vehicle:             data.Vehicle,
		LatestReading:       latestMiles,
		LatestDate:          latestDate,
		TargetToday:         targetToday,
		Delta:               delta,
		PercentUsed:         pctUsed,
		DaysLeftYear:        int(math.Ceil(daysLeftYear)),
		MilesLeftYear:       milesLeftYear,
		DaysLeftTerm:        daysLeft,
		MilesLeftTerm:       milesLeftTerm,
		YearsLeftTerm:       yearsLeft,
		DailyRate:           dailyRate,
		AvgAnnualMileage:    avgAnnualMileage,
		RecentAnnualMileage: recentAnnualMileage,
		ProjectedEnd:        projectedEnd,
		ProjectedOver:       projectedOver,
		PlanStart:           data.Plan.Start,
		PlanEnd:             data.Plan.End,
		AnnualAllowance:     data.Plan.AnnualAllowance,
		StartMiles:          data.Plan.StartMiles,

		DaysToEnd:             daysToEnd,
		EstimatedFinalMileage: estimatedFinalMileage,
		DrivableDailyRate:     drivableDailyRate,
		ExcessRate:            data.Plan.ExcessRate,
		ProjectedExcessMiles:  projectedExcessMiles,
		ProjectedOverageCost:  projectedOverageCost,
		PaceTrendDelta:        paceTrendDelta,
		PaceTrend:             paceTrend,
	}
}
