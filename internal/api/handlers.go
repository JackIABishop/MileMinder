package api

import (
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/jackiabishop/mileminder/internal/calc"
	"github.com/jackiabishop/mileminder/internal/model"
	"github.com/jackiabishop/mileminder/internal/readings"
	"github.com/jackiabishop/mileminder/internal/storage"
)

// Server holds the HTTP layer's handler methods. The storage.Store is no longer
// a field: it is a per-request dependency the mode middleware injects into the
// request context (see middleware.go, storeFrom), so the same handlers serve
// both the single-user store and a hosted user's scoped store without knowing
// which. Handlers read it with storeFrom(r.Context()).
type Server struct{}

// NewServer returns a Server.
func NewServer() *Server {
	return &Server{}
}

type apiErrorResponse struct {
	Error apiError `json:"error"`
}

type apiError struct {
	Code    string `json:"code"`
	Message string `json:"message"`
	// Details carries per-row problems for CSV import; empty elsewhere.
	Details []readings.RowError `json:"details,omitempty"`
}

func writeValidationError(w http.ResponseWriter, code, message string) {
	writeValidationErrorDetails(w, code, message, nil)
}

func writeValidationErrorDetails(w http.ResponseWriter, code, message string, details []readings.RowError) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusBadRequest)
	json.NewEncoder(w).Encode(apiErrorResponse{
		Error: apiError{
			Code:    code,
			Message: message,
			Details: details,
		},
	})
}

type wholePence int

func (p *wholePence) UnmarshalJSON(b []byte) error {
	if string(b) == "null" {
		return nil
	}
	f, err := strconv.ParseFloat(string(b), 64)
	if err != nil {
		return errors.New("excess_rate must be a whole number of pence")
	}
	if math.IsNaN(f) || math.IsInf(f, 0) || math.Trunc(f) != f {
		return errors.New("excess_rate must be a whole number of pence")
	}
	*p = wholePence(f)
	return nil
}

// writeStoreError maps a storage error onto an HTTP response: a missing
// vehicle/reading (storage.ErrNotFound) becomes a clean 404 without leaking
// internal detail; anything else is a genuine I/O failure and becomes a 500.
func writeStoreError(w http.ResponseWriter, err error) {
	if errors.Is(err, storage.ErrNotFound) {
		http.Error(w, "not found", http.StatusNotFound)
		return
	}
	http.Error(w, err.Error(), http.StatusInternalServerError)
}

// VehicleListItem represents a vehicle in the list response
type VehicleListItem struct {
	ID        string `json:"id"`
	Vehicle   string `json:"vehicle"`
	IsDefault bool   `json:"is_default"`
}

// VehicleStatus is the computed status for a vehicle. The canonical type and
// all status math now live in internal/calc; this alias keeps the API handlers
// and JSON contract unchanged.
type VehicleStatus = calc.Status

// FleetResponse is the GET /api/v1/fleet envelope: the per-vehicle statuses plus a
// household roll-up derived from those same statuses (see calc.ComputeFleetInsights).
type FleetResponse struct {
	Vehicles []VehicleStatus    `json:"vehicles"`
	Insights calc.FleetInsights `json:"insights"`
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
func (s *Server) HandleListVehicles(w http.ResponseWriter, r *http.Request) {
	records, err := storeFrom(r.Context()).ListVehicles(r.Context())
	if err != nil {
		writeStoreError(w, err)
		return
	}

	defaultID, err := storeFrom(r.Context()).GetCurrent(r.Context())
	if err != nil {
		writeStoreError(w, err)
		return
	}

	vehicles := []VehicleListItem{}
	for _, rec := range records {
		vehicles = append(vehicles, VehicleListItem{
			ID:        rec.ID,
			Vehicle:   rec.Data.Vehicle,
			IsDefault: rec.ID == defaultID,
		})
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(vehicles)
}

// HandleGetVehicle returns details and status for a specific vehicle
func (s *Server) HandleGetVehicle(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if id == "" {
		http.Error(w, "vehicle ID required", http.StatusBadRequest)
		return
	}

	data, err := storeFrom(r.Context()).GetVehicle(r.Context(), id)
	if err != nil {
		writeStoreError(w, err)
		return
	}

	status := calc.ComputeStatus(id, data)

	defaultID, err := storeFrom(r.Context()).GetCurrent(r.Context())
	if err != nil {
		writeStoreError(w, err)
		return
	}
	status.IsDefault = defaultID == id

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(status)
}

// HandleCreateVehicle creates a new vehicle.
//
// SaveVehicle is an upsert, so this currently overwrites an existing vehicle of
// the same id (unchanged from prior behaviour). Rejecting duplicates with a 409
// is tracked separately in #31.
func (s *Server) HandleCreateVehicle(w http.ResponseWriter, r *http.Request) {
	var req struct {
		ID              string     `json:"id"`
		Vehicle         string     `json:"vehicle"`
		StartDate       string     `json:"start_date"`
		EndDate         string     `json:"end_date"`
		AnnualAllowance *int       `json:"annual_allowance"`
		StartMiles      int        `json:"start_miles"`
		ExcessRate      wholePence `json:"excess_rate"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		if strings.Contains(err.Error(), "excess_rate") {
			writeValidationError(w, "invalid_excess_rate", "excess_rate must be a whole number of pence")
		} else {
			writeValidationError(w, "invalid_json", err.Error())
		}
		return
	}

	hasPlanFields := req.EndDate != "" || req.AnnualAllowance != nil
	if hasPlanFields && (req.StartDate == "" || req.EndDate == "" || req.AnnualAllowance == nil) {
		writeValidationError(w, "incomplete_plan", "provide start_date, end_date and annual_allowance together, or none for a plan-less vehicle")
		return
	}
	if req.ExcessRate < 0 {
		writeValidationError(w, "invalid_excess_rate", "excess_rate must not be negative")
		return
	}

	if req.StartDate == "" {
		req.StartDate = time.Now().Format("2006-01-02")
	}
	startDate, err := time.Parse("2006-01-02", req.StartDate)
	if err != nil {
		writeValidationError(w, "invalid_start_date", "invalid start_date")
		return
	}

	data := &model.VehicleData{
		Vehicle: req.Vehicle,
		Readings: map[string]int{
			req.StartDate: req.StartMiles,
		},
	}
	if hasPlanFields {
		endDate, err := time.Parse("2006-01-02", req.EndDate)
		if err != nil {
			writeValidationError(w, "invalid_end_date", "invalid end_date")
			return
		}
		data.Plan = &model.Plan{
			Start:           startDate,
			End:             endDate,
			AnnualAllowance: *req.AnnualAllowance,
			StartMiles:      req.StartMiles,
			ExcessRate:      int(req.ExcessRate),
		}
	}

	if err := storeFrom(r.Context()).SaveVehicle(r.Context(), req.ID, data); err != nil {
		writeStoreError(w, err)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	json.NewEncoder(w).Encode(map[string]string{"status": "created", "id": req.ID})
}

// HandleUpdatePlan applies a partial update to a vehicle's plan. Only the fields
// present in the request body are changed; everything else is preserved. Today
// this exists primarily so an excess_rate can be set on a vehicle that already
// exists (it can't only be settable at creation time).
func (s *Server) HandleUpdatePlan(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if id == "" {
		http.Error(w, "vehicle ID required", http.StatusBadRequest)
		return
	}

	// Pointer fields so an omitted key leaves the existing value untouched
	// (rather than zeroing it).
	var req struct {
		ExcessRate      *wholePence `json:"excess_rate"`
		StartDate       *string     `json:"start_date"`
		EndDate         *string     `json:"end_date"`
		AnnualAllowance *int        `json:"annual_allowance"`
		StartMiles      *int        `json:"start_miles"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		if strings.Contains(err.Error(), "excess_rate") {
			writeValidationError(w, "invalid_excess_rate", "excess_rate must be a whole number of pence")
		} else {
			writeValidationError(w, "invalid_json", err.Error())
		}
		return
	}

	data, err := storeFrom(r.Context()).GetVehicle(r.Context(), id)
	if err != nil {
		writeStoreError(w, err)
		return
	}

	hasConversionFields := req.StartDate != nil || req.EndDate != nil || req.AnnualAllowance != nil || req.StartMiles != nil
	if hasConversionFields {
		if data.Plan != nil {
			writeValidationError(w, "vehicle_already_has_plan", "vehicle already has a plan")
			return
		}
		if req.StartDate == nil || req.EndDate == nil || req.AnnualAllowance == nil || req.StartMiles == nil {
			writeValidationError(w, "incomplete_plan", "provide start_date, end_date, annual_allowance and start_miles together")
			return
		}
		startDate, err := time.Parse("2006-01-02", *req.StartDate)
		if err != nil {
			writeValidationError(w, "invalid_start_date", "invalid start_date")
			return
		}
		endDate, err := time.Parse("2006-01-02", *req.EndDate)
		if err != nil {
			writeValidationError(w, "invalid_end_date", "invalid end_date")
			return
		}
		excessRate := 0
		if req.ExcessRate != nil {
			excessRate = int(*req.ExcessRate)
		}
		if excessRate < 0 {
			writeValidationError(w, "invalid_excess_rate", "excess_rate must not be negative")
			return
		}
		data.Plan = &model.Plan{
			Start:           startDate,
			End:             endDate,
			AnnualAllowance: *req.AnnualAllowance,
			StartMiles:      *req.StartMiles,
			ExcessRate:      excessRate,
		}
	} else if req.ExcessRate != nil {
		if data.Plan == nil {
			writeValidationError(w, "vehicle_has_no_plan", "vehicle has no allowance plan")
			return
		}
		if *req.ExcessRate < 0 {
			writeValidationError(w, "invalid_excess_rate", "excess_rate must not be negative")
			return
		}
		data.Plan.ExcessRate = int(*req.ExcessRate)
	}

	if err := storeFrom(r.Context()).SaveVehicle(r.Context(), id, data); err != nil {
		writeStoreError(w, err)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{"status": "updated", "id": id})
}

// HandleAddReading adds a new odometer reading
func (s *Server) HandleAddReading(w http.ResponseWriter, r *http.Request) {
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

	data, err := storeFrom(r.Context()).GetVehicle(r.Context(), id)
	if err != nil {
		writeStoreError(w, err)
		return
	}

	// Validate against max existing reading (shared rule, per-surface message).
	if max, below := readings.BelowMax(data.Readings, req.Miles); below && !req.Force {
		http.Error(w, fmt.Sprintf("new reading %d is less than existing max %d; set force=true to override", req.Miles, max), http.StatusBadRequest)
		return
	}

	if err := storeFrom(r.Context()).PutReading(r.Context(), id, req.Date, req.Miles); err != nil {
		writeStoreError(w, err)
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
func (s *Server) HandleGetReadings(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if id == "" {
		http.Error(w, "vehicle ID required", http.StatusBadRequest)
		return
	}

	data, err := storeFrom(r.Context()).GetVehicle(r.Context(), id)
	if err != nil {
		writeStoreError(w, err)
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
func (s *Server) HandleDeleteReading(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	date := r.PathValue("date")
	if id == "" || date == "" {
		http.Error(w, "vehicle ID and date required", http.StatusBadRequest)
		return
	}

	if err := storeFrom(r.Context()).DeleteReading(r.Context(), id, date); err != nil {
		writeStoreError(w, err)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{"status": "deleted"})
}

// HandleGetGraphData returns data formatted for graphing
func (s *Server) HandleGetGraphData(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if id == "" {
		http.Error(w, "vehicle ID required", http.StatusBadRequest)
		return
	}

	data, err := storeFrom(r.Context()).GetVehicle(r.Context(), id)
	if err != nil {
		writeStoreError(w, err)
		return
	}

	dates := make([]string, 0, len(data.Readings))
	for d := range data.Readings {
		dates = append(dates, d)
	}
	sort.Strings(dates)

	var actuals []float64
	ideals := []float64{}
	baseMiles := 0.0
	if data.Plan != nil {
		baseMiles = float64(data.Plan.StartMiles)
	} else if len(dates) > 0 {
		baseMiles = float64(data.Readings[dates[0]])
	}

	for _, ds := range dates {
		t, _ := time.Parse("2006-01-02", ds)
		miles := float64(data.Readings[ds]) - baseMiles
		actuals = append(actuals, miles)
		if data.Plan != nil {
			ideals = append(ideals, calc.AllowanceMiles(data.Plan.AnnualAllowance, data.Plan.Start, t))
		}
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(GraphData{
		Dates:   dates,
		Actuals: actuals,
		Ideals:  ideals,
	})
}

// HandleGetCurrent returns the current default vehicle
func (s *Server) HandleGetCurrent(w http.ResponseWriter, r *http.Request) {
	current, err := storeFrom(r.Context()).GetCurrent(r.Context())
	if err != nil {
		writeStoreError(w, err)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{"current": current})
}

// HandleSetCurrent sets the current default vehicle
func (s *Server) HandleSetCurrent(w http.ResponseWriter, r *http.Request) {
	var req struct {
		ID string `json:"id"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	if err := storeFrom(r.Context()).SetCurrent(r.Context(), req.ID); err != nil {
		writeStoreError(w, err)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{"status": "updated", "current": req.ID})
}

// HandleFleet returns status for all vehicles
func (s *Server) HandleFleet(w http.ResponseWriter, r *http.Request) {
	records, err := storeFrom(r.Context()).ListVehicles(r.Context())
	if err != nil {
		writeStoreError(w, err)
		return
	}

	defaultID, err := storeFrom(r.Context()).GetCurrent(r.Context())
	if err != nil {
		writeStoreError(w, err)
		return
	}

	// Always serialise an empty array (not null) for Vehicles so the shape is
	// stable for clients.
	fleet := []VehicleStatus{}
	for _, rec := range records {
		status := calc.ComputeStatus(rec.ID, rec.Data)
		status.IsDefault = rec.ID == defaultID
		fleet = append(fleet, status)
	}

	resp := FleetResponse{
		Vehicles: fleet,
		Insights: calc.ComputeFleetInsights(fleet),
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(resp)
}

// maxImportBytes caps the import request body. Years of daily readings is
// ~20 KB, so 1 MB is generous while still bounding hosted-mode uploads.
const maxImportBytes = 1 << 20

// HandleImportCSV bulk-imports readings from a CSV body in the exact format
// HandleExportCSV writes (round-trip guarantee). All-or-nothing: any invalid
// row rejects the whole file with every error line-numbered. Existing dates
// are skipped unless ?overwrite=true; the merged set must be monotonic by
// date unless ?force=true. One SaveVehicle write keeps the import atomic.
func (s *Server) HandleImportCSV(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if id == "" {
		http.Error(w, "vehicle ID required", http.StatusBadRequest)
		return
	}
	overwrite := r.URL.Query().Get("overwrite") == "true"
	force := r.URL.Query().Get("force") == "true"

	data, err := storeFrom(r.Context()).GetVehicle(r.Context(), id)
	if err != nil {
		writeStoreError(w, err)
		return
	}

	rows, rowErrs := readings.ParseCSV(http.MaxBytesReader(w, r.Body, maxImportBytes))
	if len(rowErrs) > 0 {
		writeValidationErrorDetails(w, "invalid_csv",
			fmt.Sprintf("CSV has %d invalid row(s); nothing was imported", len(rowErrs)), rowErrs)
		return
	}

	merged, report := readings.Merge(data.Readings, rows, overwrite)
	if !force {
		if err := readings.CheckMonotonic(merged); err != nil {
			writeValidationError(w, "not_monotonic", err.Error()+"; set force=true to override")
			return
		}
	}

	data.Readings = merged
	if err := storeFrom(r.Context()).SaveVehicle(r.Context(), id, data); err != nil {
		writeStoreError(w, err)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"status":      "imported",
		"added":       report.Added,
		"skipped":     report.Skipped,
		"overwritten": report.Overwritten,
	})
}

// HandleExportCSV exports readings as CSV
func (s *Server) HandleExportCSV(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if id == "" {
		http.Error(w, "vehicle ID required", http.StatusBadRequest)
		return
	}

	data, err := storeFrom(r.Context()).GetVehicle(r.Context(), id)
	if err != nil {
		writeStoreError(w, err)
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
