package api

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"sort"
	"time"

	"github.com/jackiabishop/mileminder/internal/calc"
	"github.com/jackiabishop/mileminder/internal/model"
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
		ID              string `json:"id"`
		Vehicle         string `json:"vehicle"`
		StartDate       string `json:"start_date"`
		EndDate         string `json:"end_date"`
		AnnualAllowance int    `json:"annual_allowance"`
		StartMiles      int    `json:"start_miles"`
		ExcessRate      int    `json:"excess_rate"`
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
			ExcessRate:      req.ExcessRate,
		},
		Readings: map[string]int{
			req.StartDate: req.StartMiles,
		},
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
		ExcessRate *int `json:"excess_rate"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	data, err := storeFrom(r.Context()).GetVehicle(r.Context(), id)
	if err != nil {
		writeStoreError(w, err)
		return
	}

	if req.ExcessRate != nil {
		if *req.ExcessRate < 0 {
			http.Error(w, "excess_rate must not be negative", http.StatusBadRequest)
			return
		}
		data.Plan.ExcessRate = *req.ExcessRate
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
	var ideals []float64
	baseMiles := float64(data.Plan.StartMiles)

	for _, ds := range dates {
		t, _ := time.Parse("2006-01-02", ds)
		miles := float64(data.Readings[ds]) - baseMiles
		actuals = append(actuals, miles)
		ideals = append(ideals, calc.AllowanceMiles(data.Plan.AnnualAllowance, data.Plan.Start, t))
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
