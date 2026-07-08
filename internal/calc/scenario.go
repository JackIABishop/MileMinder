package calc

import (
	"errors"
	"math"
	"time"

	"github.com/jackiabishop/mileminder/internal/model"
)

// Scenario is the result of a read-only "what if" projection: it answers
// "if I drive extraMiles on top of my normal trajectory by byDate, where do I
// land relative to my allowance?". It never mutates the input vehicle.
//
// The projection is additive to the vehicle's existing trajectory, not a
// replacement for it: BaselineMiles is where the current daily pace alone would
// put the odometer at byDate, and HypotheticalMiles adds the extra trip miles
// on top. Status is the full status computed *as of byDate* — so Delta,
// PercentUsed and the year/segment figures are the snapshot you'd see on that
// date having taken the trip. The projection-family fields on Status
// (ProjectedEnd, EstimatedFinalMileage, ProjectedOverageCost*) assume the
// scenario's elevated pace continues, so clients should label them accordingly.
type Scenario struct {
	ExtraMiles        float64 `json:"extra_miles"`
	ByDate            string  `json:"by_date"`            // YYYY-MM-DD, echoed back
	BaselineMiles     float64 `json:"baseline_miles"`     // projected odometer at byDate without the trip
	HypotheticalMiles float64 `json:"hypothetical_miles"` // baseline + extra — the overlay endpoint
	Status            Status  `json:"status"`             // status of the hypothetical, as of byDate
}

// Domain-rule errors from ComputeScenario. Callers (e.g. the API layer) can map
// these onto 400-class responses via errors.Is.
var (
	ErrScenarioNoPlan        = errors.New("scenario requires an allowance plan")
	ErrScenarioNoReadings    = errors.New("scenario requires at least one reading")
	ErrScenarioDateNotFuture = errors.New("by_date must be after today and after the latest reading")
	ErrScenarioAfterPlanEnd  = errors.New("by_date must be on or before the plan end")
	ErrScenarioNegativeMiles = errors.New("extra_miles must not be negative")
)

// ComputeScenario runs a what-if projection for a vehicle as of now. It is
// read-only: the caller's data is never modified.
func ComputeScenario(id string, data *model.VehicleData, extraMiles float64, byDate time.Time) (Scenario, error) {
	return computeScenario(id, data, extraMiles, byDate, time.Now())
}

// computeScenario is the deterministic core. `now` is injected so the math is
// testable against a fixed clock, mirroring computeStatus.
func computeScenario(id string, data *model.VehicleData, extraMiles float64, byDate, now time.Time) (Scenario, error) {
	if !data.HasPlan() {
		return Scenario{}, ErrScenarioNoPlan
	}
	if extraMiles < 0 {
		return Scenario{}, ErrScenarioNegativeMiles
	}

	readings := SortedReadings(data)
	if len(readings) == 0 {
		return Scenario{}, ErrScenarioNoReadings
	}
	latest := readings[len(readings)-1]

	// Normalise `now` and byDate to date granularity so the "must be in the
	// future" rule compares dates, not wall-clock instants (matching how
	// readings are keyed by local calendar day).
	today, _ := time.Parse("2006-01-02", now.Format("2006-01-02"))
	byDate, _ = time.Parse("2006-01-02", byDate.Format("2006-01-02"))
	if !byDate.After(today) || !byDate.After(latest.Date) {
		return Scenario{}, ErrScenarioDateNotFuture
	}
	if byDate.After(data.Plan.End) {
		return Scenario{}, ErrScenarioAfterPlanEnd
	}

	// Baseline: continue the current allowance-year pace from the latest reading
	// to byDate. This is the same daily_rate the status projection already uses,
	// so the what-if is additive to the trajectory the user already sees.
	current := computeStatus(id, data, now)
	days := byDate.Sub(latest.Date).Hours() / 24.0
	baseline := latest.Miles + current.DailyRate*days
	hypothetical := baseline + extraMiles

	// Build a copy of the vehicle with the synthetic reading. Never mutate the
	// caller's data — the CLI passes live structs, and calc owns no persistence.
	hypData := *data
	hypData.Readings = make(map[string]int, len(data.Readings)+1)
	for k, v := range data.Readings {
		hypData.Readings[k] = v
	}
	hypData.Readings[byDate.Format("2006-01-02")] = int(math.Round(hypothetical))

	// Compute the hypothetical status *as of byDate* so delta/percent-used and
	// the allowance-year segment are the snapshot for that date, not today.
	status := computeStatus(id, &hypData, byDate)

	return Scenario{
		ExtraMiles:        extraMiles,
		ByDate:            byDate.Format("2006-01-02"),
		BaselineMiles:     baseline,
		HypotheticalMiles: hypothetical,
		Status:            status,
	}, nil
}
