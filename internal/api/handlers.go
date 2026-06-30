package api

import (
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/jackiabishop/mileminder/internal/calc"
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

// VehicleStatus is the computed status for a vehicle. The canonical type and
// all status math now live in internal/calc; this alias keeps the API handlers
// and JSON contract unchanged.
type VehicleStatus = calc.Status

// FleetResponse is the GET /api/fleet envelope: the per-vehicle statuses plus a
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

	status := calc.ComputeStatus(id, data)

	// Check if default
	dir, _ := getMileMinderDir()
	if defaultData, err := os.ReadFile(filepath.Join(dir, "current")); err == nil {
		status.IsDefault = strings.TrimSpace(string(defaultData)) == id
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(status)
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

	// Always serialise an empty array (not null) for Vehicles so the shape is
	// stable for clients.
	fleet := []VehicleStatus{}

	entries, err := os.ReadDir(dir)
	if err != nil {
		if os.IsNotExist(err) {
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(FleetResponse{Vehicles: fleet, Insights: calc.ComputeFleetInsights(fleet)})
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

	for _, e := range entries {
		if e.IsDir() || filepath.Ext(e.Name()) != ".yml" {
			continue
		}
		id := strings.TrimSuffix(e.Name(), ".yml")
		v, err := loadVehicle(id)
		if err != nil {
			continue
		}
		status := calc.ComputeStatus(id, v)
		status.IsDefault = id == defaultID
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
