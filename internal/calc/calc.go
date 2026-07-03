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
	HasPlan             bool      `json:"has_plan"`
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
	ProjectedOverageCost float64 `json:"projected_overage_cost"`

	// Trend signal (#7): recent (90-day) annual pace vs lifetime average.
	// PaceTrend classifies the delta as accelerating / easing / steady.
	PaceTrendDelta float64 `json:"pace_trend_delta"`
	PaceTrend      string  `json:"pace_trend"`
}

// FleetInsights is a household-level roll-up derived purely from a slice of
// already-computed Status values. It adds no new per-vehicle math — it only
// composes calc outputs — so it stays consistent with ComputeStatus by
// construction. JSON tags mirror the web/iOS API contract.
type FleetInsights struct {
	TotalVehicles         int     `json:"total_vehicles"`
	PolicyVehicles        int     `json:"policy_vehicles"`
	PlainVehicles         int     `json:"plain_vehicles"`
	CountOver             int     `json:"count_over"`               // cars with Delta > 0
	CountUnder            int     `json:"count_under"`              // cars with Delta <= 0
	NetDelta              float64 `json:"net_delta"`                // Σ Delta (miles; +ve = collectively over the allowance line)
	TotalAvgAnnualMileage float64 `json:"total_avg_annual_mileage"` // Σ AvgAnnualMileage (household annualised miles)
	AvgPercentUsed        float64 `json:"avg_percent_used"`         // fleet mean PercentUsed — comparative-pace baseline
	WorstOffenderID       string  `json:"worst_offender_id"`        // id of the car with the highest PercentUsed ("" for an empty fleet)
	WorstOffenderVehicle  string  `json:"worst_offender_vehicle"`   // that car's display name, for the UI headline
}

// Breach explains which alert/check conditions a policy vehicle currently
// violates. Plain vehicles never breach.
type Breach struct {
	Over          bool
	ThresholdHit  bool
	ProjectedOver bool
}

// Breached reports whether any breach condition is true.
func (b Breach) Breached() bool {
	return b.Over || b.ThresholdHit || b.ProjectedOver
}

// EvaluateBreach applies MileMinder's allowance breach predicate to a computed
// status. Threshold is a percent-used value; 100 matches the CLI default.
func EvaluateBreach(s Status, threshold float64) Breach {
	if !s.HasPlan {
		return Breach{}
	}
	return Breach{
		Over:          s.Delta > 0,
		ThresholdHit:  s.PercentUsed >= threshold,
		ProjectedOver: s.ProjectedOver,
	}
}

// ComputeFleetInsights aggregates a slice of per-vehicle statuses into a
// household roll-up. "Worst offender" and comparative pace are ranked by
// PercentUsed, which normalises across vehicles with different allowances. An
// empty slice yields a zero-value result with an empty WorstOffenderID.
func ComputeFleetInsights(statuses []Status) FleetInsights {
	insights := FleetInsights{TotalVehicles: len(statuses)}
	if len(statuses) == 0 {
		return insights
	}

	worst := -1
	var worstPct float64
	var sumPct float64
	for i, s := range statuses {
		insights.TotalAvgAnnualMileage += s.AvgAnnualMileage
		if !s.HasPlan {
			insights.PlainVehicles++
			continue
		}
		insights.PolicyVehicles++
		if s.Delta > 0 {
			insights.CountOver++
		} else {
			insights.CountUnder++
		}
		insights.NetDelta += s.Delta
		sumPct += s.PercentUsed
		if worst < 0 || s.PercentUsed > worstPct {
			worst = i
			worstPct = s.PercentUsed
		}
	}

	if insights.PolicyVehicles > 0 {
		insights.AvgPercentUsed = sumPct / float64(insights.PolicyVehicles)
		insights.WorstOffenderID = statuses[worst].ID
		insights.WorstOffenderVehicle = statuses[worst].Vehicle
	}
	return insights
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

// ComputeStatusAt calculates all status metrics for a vehicle as of now. It is
// the public deterministic wrapper used by background jobs and tests.
func ComputeStatusAt(id string, data *model.VehicleData, now time.Time) Status {
	return computeStatus(id, data, now)
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
	latestMiles := 0
	if len(dates) > 0 {
		latestDate = dates[len(dates)-1]
		latestMiles = data.Readings[latestDate]
	}

	readings := SortedReadings(data)

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

	if data.Plan == nil {
		avgAnnualMileage := 0.0
		dailyRate := 0.0
		if len(readings) > 0 {
			milesUsed := float64(latestMiles) - readings[0].Miles
			if milesUsed < 0 {
				milesUsed = 0
			}
			daysTracked := today.Sub(readings[0].Date).Hours() / 24.0
			if daysTracked >= 1 {
				avgAnnualMileage = milesUsed / daysTracked * 365.0
				dailyRate = milesUsed / daysTracked
			}
		}

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
			HasPlan:             false,
			LatestReading:       latestMiles,
			LatestDate:          latestDate,
			DailyRate:           dailyRate,
			AvgAnnualMileage:    avgAnnualMileage,
			RecentAnnualMileage: recentAnnualMileage,
			PaceTrendDelta:      paceTrendDelta,
			PaceTrend:           paceTrend,
		}
	}

	plan := data.Plan
	if len(dates) == 0 {
		latestMiles = plan.StartMiles
	}
	// Compute target vs actual
	daysElapsed := today.Sub(plan.Start).Hours() / 24.0
	if daysElapsed < 0 {
		daysElapsed = 0
	}
	targetToday := float64(plan.StartMiles) + float64(plan.AnnualAllowance)*daysElapsed/365.0
	milesUsed := float64(latestMiles - plan.StartMiles)
	delta := milesUsed - (targetToday - float64(plan.StartMiles))

	var pctUsed float64
	targetMileage := targetToday - float64(plan.StartMiles)
	if targetMileage > 0 {
		pctUsed = milesUsed / targetMileage * 100.0
	}

	// Year left calculation
	yearsSince := today.Year() - plan.Start.Year()
	segmentStart := plan.Start.AddDate(yearsSince, 0, 0)
	if segmentStart.After(today) {
		segmentStart = segmentStart.AddDate(-1, 0, 0)
	}
	segmentEnd := segmentStart.AddDate(1, 0, 0)
	if segmentEnd.After(plan.End) {
		segmentEnd = plan.End
	}

	daysLeftYear := segmentEnd.Sub(today).Hours() / 24.0
	if daysLeftYear < 0 {
		daysLeftYear = 0
	}
	milesLeftYear := float64(plan.AnnualAllowance) * daysLeftYear / 365.0

	// Daily rate within the current allowance year. Interpolate the odometer
	// exactly at the segment boundary so miles driven *before* this year started
	// aren't counted against it — both numerator and denominator then cover the
	// same window (segmentStart → today).
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

	projectedUsage := dailyRate * segmentDurationDays
	allowanceSegment := float64(plan.AnnualAllowance) * segmentDurationDays / 365.0
	projectedEnd := allowanceSegment - projectedUsage
	projectedOver := projectedEnd < 0
	if projectedOver {
		projectedEnd = -projectedEnd
	}

	// Term left
	termDays := plan.End.Sub(today).Hours() / 24.0
	if termDays < 0 {
		termDays = 0
	}
	yearsLeft := int(termDays / 365.0)
	daysLeft := int(math.Mod(termDays, 365.0))
	milesLeftTerm := float64(plan.AnnualAllowance) * termDays / 365.0

	// Renewal countdown + final-mileage estimate (#3). daysToEnd is the whole
	// countdown to plan end; the final-mileage estimate continues from the
	// latest reading at the current daily pace.
	daysToEnd := int(math.Ceil(termDays))
	estimatedFinalMileage := float64(latestMiles) + dailyRate*termDays

	// Drivable-rate budget (#4): how many miles/day you can still drive for the
	// rest of the plan and finish within the total term allowance. Capacity, not
	// pace: (total term allowance − miles already used) ÷ days remaining.
	totalTermDays := plan.End.Sub(plan.Start).Hours() / 24.0
	totalTermAllowanceMiles := float64(plan.AnnualAllowance) * totalTermDays / 365.0
	drivableDailyRate := 0.0
	if termDays >= 1 {
		drivableDailyRate = (totalTermAllowanceMiles - milesUsed) / termDays
		if drivableDailyRate < 0 {
			drivableDailyRate = 0
		}
	}

	// Overage cost estimate (#5): how far the projected final mileage overshoots
	// the total term allowance, and the £ penalty at the plan's excess rate.
	projectedMilesDriven := estimatedFinalMileage - float64(plan.StartMiles)
	projectedExcessMiles := projectedMilesDriven - totalTermAllowanceMiles
	if projectedExcessMiles < 0 {
		projectedExcessMiles = 0
	}
	projectedOverageCost := 0.0
	if plan.ExcessRate > 0 {
		projectedOverageCost = projectedExcessMiles * float64(plan.ExcessRate) / 100.0
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
		HasPlan:             true,
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
		PlanStart:           plan.Start,
		PlanEnd:             plan.End,
		AnnualAllowance:     plan.AnnualAllowance,
		StartMiles:          plan.StartMiles,

		DaysToEnd:             daysToEnd,
		EstimatedFinalMileage: estimatedFinalMileage,
		DrivableDailyRate:     drivableDailyRate,
		ExcessRate:            plan.ExcessRate,
		ProjectedExcessMiles:  projectedExcessMiles,
		ProjectedOverageCost:  projectedOverageCost,
		PaceTrendDelta:        paceTrendDelta,
		PaceTrend:             paceTrend,
	}
}
