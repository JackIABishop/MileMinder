package api

import (
	"encoding/json"
	"fmt"
	"math"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/jackiabishop/mileminder/internal/model"
	"gopkg.in/yaml.v3"
)

// getMileMinderDir returns the path to ~/.mileminder/
func getMileMinderDir() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, ".mileminder"), nil
}

// loadVehicle loads a vehicle from its YAML file
func loadVehicle(id string) (*model.VehicleData, error) {
	dir, err := getMileMinderDir()
	if err != nil {
		return nil, err
	}
	raw, err := os.ReadFile(filepath.Join(dir, id+".yml"))
	if err != nil {
		return nil, err
	}
	var data model.VehicleData
	if err := yaml.Unmarshal(raw, &data); err != nil {
		return nil, err
	}
	return &data, nil
}

// saveVehicle saves a vehicle to its YAML file
func saveVehicle(id string, data *model.VehicleData) error {
	dir, err := getMileMinderDir()
	if err != nil {
		return err
	}
	if err := os.MkdirAll(dir, 0755); err != nil {
		return err
	}
	f, err := os.Create(filepath.Join(dir, id+".yml"))
	if err != nil {
		return err
	}
	defer f.Close()
	enc := yaml.NewEncoder(f)
	defer enc.Close()
	return enc.Encode(data)
}

// VehicleListItem represents a vehicle in the list response
type VehicleListItem struct {
	ID        string `json:"id"`
	Vehicle   string `json:"vehicle"`
	IsDefault bool   `json:"is_default"`
}

// VehicleStatus represents computed status for a vehicle
type VehicleStatus struct {
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
}

// Reading represents a single odometer reading
type Reading struct {
	Date  string `json:"date"`
	Miles int    `json:"miles"`
}

// GraphData represents data for the mileage graph
type GraphData struct {
	Dates   []string  `json:"dates"`
	Actuals []float64 `json:"actuals"`
	Ideals  []float64 `json:"ideals"`
}

// HandleListVehicles returns all vehicles
func HandleListVehicles(w http.ResponseWriter, r *http.Request) {
	dir, err := getMileMinderDir()
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	entries, err := os.ReadDir(dir)
	if err != nil {
		if os.IsNotExist(err) {
			json.NewEncoder(w).Encode([]VehicleListItem{})
			return
		}
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	// Read default vehicle
	defaultID := ""
	if data, err := os.ReadFile(filepath.Join(dir, "current")); err == nil {
		defaultID = strings.TrimSpace(string(data))
	}

	var vehicles []VehicleListItem
	for _, e := range entries {
		if e.IsDir() || filepath.Ext(e.Name()) != ".yml" {
			continue
		}
		id := strings.TrimSuffix(e.Name(), ".yml")
		v, err := loadVehicle(id)
		if err != nil {
			continue
		}
		vehicles = append(vehicles, VehicleListItem{
			ID:        id,
			Vehicle:   v.Vehicle,
			IsDefault: id == defaultID,
		})
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(vehicles)
}

// HandleGetVehicle returns details and status for a specific vehicle
func HandleGetVehicle(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if id == "" {
		http.Error(w, "vehicle ID required", http.StatusBadRequest)
		return
	}

	data, err := loadVehicle(id)
	if err != nil {
		http.Error(w, err.Error(), http.StatusNotFound)
		return
	}

	status := computeStatus(id, data)

	// Check if default
	dir, _ := getMileMinderDir()
	if defaultData, err := os.ReadFile(filepath.Join(dir, "current")); err == nil {
		status.IsDefault = strings.TrimSpace(string(defaultData)) == id
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(status)
}

// datedReading is a single odometer reading with a parsed date.
type datedReading struct {
	date  time.Time
	miles float64
}

// sortedReadings returns the vehicle's readings parsed and sorted by date.
func sortedReadings(data *model.VehicleData) []datedReading {
	out := make([]datedReading, 0, len(data.Readings))
	for ds, m := range data.Readings {
		if t, err := time.Parse("2006-01-02", ds); err == nil {
			out = append(out, datedReading{date: t, miles: float64(m)})
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].date.Before(out[j].date) })
	return out
}

// odometerAt estimates the odometer reading at time `at` by linearly
// interpolating between the two readings that bracket it. Outside the data
// range it clamps to the first/last reading. Returns false if there are no
// readings to work from.
func odometerAt(rs []datedReading, at time.Time) (float64, bool) {
	if len(rs) == 0 {
		return 0, false
	}
	if !at.After(rs[0].date) {
		return rs[0].miles, true
	}
	if !at.Before(rs[len(rs)-1].date) {
		return rs[len(rs)-1].miles, true
	}
	for i := 1; i < len(rs); i++ {
		if !at.After(rs[i].date) {
			a, b := rs[i-1], rs[i]
			span := b.date.Sub(a.date).Seconds()
			if span <= 0 {
				return b.miles, true
			}
			frac := at.Sub(a.date).Seconds() / span
			return a.miles + (b.miles-a.miles)*frac, true
		}
	}
	return rs[len(rs)-1].miles, true
}

// computeStatus calculates all status metrics for a vehicle
func computeStatus(id string, data *model.VehicleData) VehicleStatus {
	today := time.Now()

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
	readings := sortedReadings(data)
	segmentDurationDays := segmentEnd.Sub(segmentStart).Hours() / 24.0
	daysSoFar := today.Sub(segmentStart).Hours() / 24.0
	if daysSoFar < 1 {
		daysSoFar = 1
	}
	milesSoFar := 0.0
	if segMiles, ok := odometerAt(readings, segmentStart); ok {
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
		if windowStart.Before(readings[0].date) {
			windowStart = readings[0].date
		}
		windowDays := today.Sub(windowStart).Hours() / 24.0
		if windowDays >= 1 {
			if baseMiles, ok := odometerAt(readings, windowStart); ok {
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

	return VehicleStatus{
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
	}
}

// HandleCreateVehicle creates a new vehicle
func HandleCreateVehicle(w http.ResponseWriter, r *http.Request) {
	var req struct {
		ID              string `json:"id"`
		Vehicle         string `json:"vehicle"`
		StartDate       string `json:"start_date"`
		EndDate         string `json:"end_date"`
		AnnualAllowance int    `json:"annual_allowance"`
		StartMiles      int    `json:"start_miles"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	startDate, err := time.Parse("2006-01-02", req.StartDate)
	if err != nil {
		http.Error(w, "invalid start_date", http.StatusBadRequest)
		return
	}
	endDate, err := time.Parse("2006-01-02", req.EndDate)
	if err != nil {
		http.Error(w, "invalid end_date", http.StatusBadRequest)
		return
	}

	data := &model.VehicleData{
		Vehicle: req.Vehicle,
		Plan: model.Plan{
			Start:           startDate,
			End:             endDate,
			AnnualAllowance: req.AnnualAllowance,
			StartMiles:      req.StartMiles,
		},
		Readings: map[string]int{
			req.StartDate: req.StartMiles,
		},
	}

	if err := saveVehicle(req.ID, data); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	json.NewEncoder(w).Encode(map[string]string{"status": "created", "id": req.ID})
}

// HandleAddReading adds a new odometer reading
func HandleAddReading(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if id == "" {
		http.Error(w, "vehicle ID required", http.StatusBadRequest)
		return
	}

	var req struct {
		Date  string `json:"date"`
		Miles int    `json:"miles"`
		Force bool   `json:"force"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	if req.Date == "" {
		req.Date = time.Now().Format("2006-01-02")
	}

	data, err := loadVehicle(id)
	if err != nil {
		http.Error(w, err.Error(), http.StatusNotFound)
		return
	}

	// Validate against max existing reading
	maxMiles := 0
	for _, m := range data.Readings {
		if m > maxMiles {
			maxMiles = m
		}
	}
	if req.Miles < maxMiles && !req.Force {
		http.Error(w, fmt.Sprintf("new reading %d is less than existing max %d; set force=true to override", req.Miles, maxMiles), http.StatusBadRequest)
		return
	}

	data.Readings[req.Date] = req.Miles

	if err := saveVehicle(id, data); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"status": "recorded",
		"date":   req.Date,
		"miles":  req.Miles,
	})
}

// HandleGetReadings returns all readings for a vehicle
func HandleGetReadings(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if id == "" {
		http.Error(w, "vehicle ID required", http.StatusBadRequest)
		return
	}

	data, err := loadVehicle(id)
	if err != nil {
		http.Error(w, err.Error(), http.StatusNotFound)
		return
	}

	var readings []Reading
	for date, miles := range data.Readings {
		readings = append(readings, Reading{Date: date, Miles: miles})
	}
	sort.Slice(readings, func(i, j int) bool {
		return readings[i].Date < readings[j].Date
	})

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(readings)
}

// HandleDeleteReading deletes a reading for a specific date
func HandleDeleteReading(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	date := r.PathValue("date")
	if id == "" || date == "" {
		http.Error(w, "vehicle ID and date required", http.StatusBadRequest)
		return
	}

	data, err := loadVehicle(id)
	if err != nil {
		http.Error(w, err.Error(), http.StatusNotFound)
		return
	}

	if _, exists := data.Readings[date]; !exists {
		http.Error(w, "reading not found", http.StatusNotFound)
		return
	}

	delete(data.Readings, date)

	if err := saveVehicle(id, data); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{"status": "deleted"})
}

// HandleGetGraphData returns data formatted for graphing
func HandleGetGraphData(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if id == "" {
		http.Error(w, "vehicle ID required", http.StatusBadRequest)
		return
	}

	data, err := loadVehicle(id)
	if err != nil {
		http.Error(w, err.Error(), http.StatusNotFound)
		return
	}

	dates := make([]string, 0, len(data.Readings))
	for d := range data.Readings {
		dates = append(dates, d)
	}
	sort.Strings(dates)

	var actuals []float64
	var ideals []float64
	start := data.Plan.Start
	annual := float64(data.Plan.AnnualAllowance)
	baseMiles := float64(data.Plan.StartMiles)

	for _, ds := range dates {
		t, _ := time.Parse("2006-01-02", ds)
		miles := float64(data.Readings[ds]) - baseMiles
		actuals = append(actuals, miles)
		daysElapsed := t.Sub(start).Hours() / 24.0
		if daysElapsed < 0 {
			daysElapsed = 0
		}
		ideal := annual * daysElapsed / 365.0
		ideals = append(ideals, ideal)
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(GraphData{
		Dates:   dates,
		Actuals: actuals,
		Ideals:  ideals,
	})
}

// HandleGetCurrent returns the current default vehicle
func HandleGetCurrent(w http.ResponseWriter, r *http.Request) {
	dir, err := getMileMinderDir()
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	data, err := os.ReadFile(filepath.Join(dir, "current"))
	if err != nil {
		if os.IsNotExist(err) {
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(map[string]string{"current": ""})
			return
		}
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{"current": strings.TrimSpace(string(data))})
}

// HandleSetCurrent sets the current default vehicle
func HandleSetCurrent(w http.ResponseWriter, r *http.Request) {
	var req struct {
		ID string `json:"id"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	dir, err := getMileMinderDir()
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	// Verify vehicle exists
	if _, err := os.Stat(filepath.Join(dir, req.ID+".yml")); os.IsNotExist(err) {
		http.Error(w, "vehicle not found", http.StatusNotFound)
		return
	}

	if err := os.WriteFile(filepath.Join(dir, "current"), []byte(req.ID), 0644); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{"status": "updated", "current": req.ID})
}

// HandleFleet returns status for all vehicles
func HandleFleet(w http.ResponseWriter, r *http.Request) {
	dir, err := getMileMinderDir()
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	entries, err := os.ReadDir(dir)
	if err != nil {
		if os.IsNotExist(err) {
			json.NewEncoder(w).Encode([]VehicleStatus{})
			return
		}
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	// Read default vehicle
	defaultID := ""
	if data, err := os.ReadFile(filepath.Join(dir, "current")); err == nil {
		defaultID = strings.TrimSpace(string(data))
	}

	var fleet []VehicleStatus
	for _, e := range entries {
		if e.IsDir() || filepath.Ext(e.Name()) != ".yml" {
			continue
		}
		id := strings.TrimSuffix(e.Name(), ".yml")
		v, err := loadVehicle(id)
		if err != nil {
			continue
		}
		status := computeStatus(id, v)
		status.IsDefault = id == defaultID
		fleet = append(fleet, status)
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(fleet)
}

// HandleExportCSV exports readings as CSV
func HandleExportCSV(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if id == "" {
		http.Error(w, "vehicle ID required", http.StatusBadRequest)
		return
	}

	data, err := loadVehicle(id)
	if err != nil {
		http.Error(w, err.Error(), http.StatusNotFound)
		return
	}

	var readings []Reading
	for date, miles := range data.Readings {
		readings = append(readings, Reading{Date: date, Miles: miles})
	}
	sort.Slice(readings, func(i, j int) bool {
		return readings[i].Date < readings[j].Date
	})

	w.Header().Set("Content-Type", "text/csv")
	w.Header().Set("Content-Disposition", fmt.Sprintf("attachment; filename=%s_readings.csv", id))

	fmt.Fprintln(w, "date,miles")
	for _, r := range readings {
		fmt.Fprintf(w, "%s,%d\n", r.Date, r.Miles)
	}
}
