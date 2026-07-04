package api_test

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"reflect"
	"testing"
	"time"

	"github.com/jackiabishop/mileminder/internal/api"
	"github.com/jackiabishop/mileminder/internal/model"
	"github.com/jackiabishop/mileminder/internal/storage"
)

// newTestServer returns an httptest server backed by an in-memory store,
// pre-seeded with the given vehicles.
func newTestServer(t *testing.T, seed map[string]*model.VehicleData) (*httptest.Server, storage.Store) {
	t.Helper()
	st := storage.NewMemory()
	for id, data := range seed {
		if err := st.SaveVehicle(context.Background(), id, data); err != nil {
			t.Fatalf("seed %q: %v", id, err)
		}
	}
	srv := httptest.NewServer(api.NewRouter(st, ""))
	t.Cleanup(srv.Close)
	return srv, st
}

func sampleVehicle() *model.VehicleData {
	return &model.VehicleData{
		Vehicle: "Golf",
		Plan: &model.Plan{
			Start:           time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC),
			End:             time.Date(2028, 1, 1, 0, 0, 0, 0, time.UTC),
			AnnualAllowance: 10000,
			StartMiles:      5000,
		},
		Readings: map[string]int{"2025-01-01": 5000},
	}
}

func TestCreatePlainVehicle(t *testing.T) {
	srv, st := newTestServer(t, nil)

	resp, err := http.Post(srv.URL+"/api/v1/vehicles", "application/json", bytes.NewBufferString(`{
		"id":"owned",
		"vehicle":"Owned Car",
		"start_date":"2025-01-01",
		"start_miles":10000
	}`))
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("want 201, got %d", resp.StatusCode)
	}
	data, err := st.GetVehicle(context.Background(), "owned")
	if err != nil {
		t.Fatal(err)
	}
	if data.Plan != nil {
		t.Fatalf("plain vehicle stored with plan: %+v", data.Plan)
	}
	if data.Readings["2025-01-01"] != 10000 {
		t.Fatalf("initial reading not stored: %+v", data.Readings)
	}

	statusResp, err := http.Get(srv.URL + "/api/v1/vehicles/owned")
	if err != nil {
		t.Fatal(err)
	}
	defer statusResp.Body.Close()
	var status api.VehicleStatus
	if err := json.NewDecoder(statusResp.Body).Decode(&status); err != nil {
		t.Fatal(err)
	}
	if status.HasPlan {
		t.Fatal("created plain vehicle status has_plan=true")
	}
}

func TestCreateVehiclePartialPlanRejected(t *testing.T) {
	srv, _ := newTestServer(t, nil)

	resp, err := http.Post(srv.URL+"/api/v1/vehicles", "application/json", bytes.NewBufferString(`{
		"id":"bad",
		"vehicle":"Bad",
		"start_date":"2025-01-01",
		"annual_allowance":10000,
		"start_miles":10000
	}`))
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("want 400, got %d", resp.StatusCode)
	}
}

func TestCreateVehicleDecimalExcessRateRejectedCleanly(t *testing.T) {
	srv, _ := newTestServer(t, nil)

	resp, err := http.Post(srv.URL+"/api/v1/vehicles", "application/json", bytes.NewBufferString(`{
		"id":"bad",
		"vehicle":"Bad",
		"start_date":"2025-01-01",
		"end_date":"2026-01-01",
		"annual_allowance":10000,
		"start_miles":10000,
		"excess_rate":0.1
	}`))
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("want 400, got %d", resp.StatusCode)
	}
	var body struct {
		Error struct {
			Code    string `json:"code"`
			Message string `json:"message"`
		} `json:"error"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		t.Fatal(err)
	}
	if body.Error.Code != "invalid_excess_rate" || body.Error.Message != "excess_rate must be a whole number of pence" {
		t.Fatalf("unexpected error body: %+v", body)
	}
}

func TestPatchExcessRateOnPlainRejected(t *testing.T) {
	srv, _ := newTestServer(t, map[string]*model.VehicleData{
		"owned": {Vehicle: "Owned Car", Readings: map[string]int{"2025-01-01": 10000}},
	})

	req, _ := http.NewRequest(http.MethodPatch, srv.URL+"/api/v1/vehicles/owned", bytes.NewBufferString(`{"excess_rate":10}`))
	req.Header.Set("Content-Type", "application/json")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("want 400, got %d", resp.StatusCode)
	}
}

func TestPatchPlainVehicleConversion(t *testing.T) {
	srv, st := newTestServer(t, map[string]*model.VehicleData{
		"owned": {Vehicle: "Owned Car", Readings: map[string]int{"2025-01-01": 10000, "2025-02-01": 10500}},
	})

	req, _ := http.NewRequest(http.MethodPatch, srv.URL+"/api/v1/vehicles/owned", bytes.NewBufferString(`{
		"start_date":"2025-01-01",
		"end_date":"2026-01-01",
		"annual_allowance":10000,
		"start_miles":10000,
		"excess_rate":10
	}`))
	req.Header.Set("Content-Type", "application/json")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("want 200, got %d", resp.StatusCode)
	}
	data, err := st.GetVehicle(context.Background(), "owned")
	if err != nil {
		t.Fatal(err)
	}
	if data.Plan == nil || data.Plan.AnnualAllowance != 10000 || data.Plan.ExcessRate != 10 {
		t.Fatalf("conversion plan not stored: %+v", data.Plan)
	}

	statusResp, err := http.Get(srv.URL + "/api/v1/vehicles/owned")
	if err != nil {
		t.Fatal(err)
	}
	defer statusResp.Body.Close()
	var status api.VehicleStatus
	if err := json.NewDecoder(statusResp.Body).Decode(&status); err != nil {
		t.Fatal(err)
	}
	if !status.HasPlan || status.AnnualAllowance != 10000 {
		t.Fatalf("converted status missing plan fields: %+v", status)
	}
}

func TestPlainVehicleGraphHasNoIdeals(t *testing.T) {
	srv, _ := newTestServer(t, map[string]*model.VehicleData{
		"owned": {Vehicle: "Owned Car", Readings: map[string]int{"2025-01-01": 10000, "2025-02-01": 10500}},
	})

	resp, err := http.Get(srv.URL + "/api/v1/vehicles/owned/graph")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("want 200, got %d", resp.StatusCode)
	}
	bodyBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatal(err)
	}
	var raw struct {
		Ideals json.RawMessage `json:"ideals"`
	}
	if err := json.Unmarshal(bodyBytes, &raw); err != nil {
		t.Fatal(err)
	}
	if string(raw.Ideals) != "[]" {
		t.Fatalf("plain graph raw ideals = %s, want []", raw.Ideals)
	}
	var graph api.GraphData
	if err := json.Unmarshal(bodyBytes, &graph); err != nil {
		t.Fatal(err)
	}
	if len(graph.Ideals) != 0 {
		t.Fatalf("plain graph ideals = %+v, want empty", graph.Ideals)
	}
	if len(graph.Actuals) != 2 || graph.Actuals[0] != 0 || graph.Actuals[1] != 500 {
		t.Fatalf("plain graph actuals = %+v, want [0 500]", graph.Actuals)
	}
}

func TestGetMissingVehicleReturns404(t *testing.T) {
	srv, _ := newTestServer(t, nil)

	resp, err := http.Get(srv.URL + "/api/v1/vehicles/ghost")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusNotFound {
		t.Fatalf("want 404, got %d", resp.StatusCode)
	}
}

func TestGetVehicleReturnsStatus(t *testing.T) {
	srv, _ := newTestServer(t, map[string]*model.VehicleData{"golf": sampleVehicle()})

	resp, err := http.Get(srv.URL + "/api/v1/vehicles/golf")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("want 200, got %d", resp.StatusCode)
	}
	var status map[string]interface{}
	if err := json.NewDecoder(resp.Body).Decode(&status); err != nil {
		t.Fatal(err)
	}
	if status["vehicle"] == nil {
		t.Fatalf("status missing vehicle field: %v", status)
	}
}

func TestAddReadingRejectsBelowMaxWithoutForce(t *testing.T) {
	srv, st := newTestServer(t, map[string]*model.VehicleData{"golf": sampleVehicle()})

	body := bytes.NewBufferString(`{"date":"2025-06-01","miles":4000}`)
	resp, err := http.Post(srv.URL+"/api/v1/vehicles/golf/readings", "application/json", body)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("below-max add: want 400, got %d", resp.StatusCode)
	}
	// The reading must not have been persisted.
	data, _ := st.GetVehicle(context.Background(), "golf")
	if _, ok := data.Readings["2025-06-01"]; ok {
		t.Fatal("rejected reading was persisted")
	}
}

func TestAddReadingBelowMaxWithForceSucceeds(t *testing.T) {
	srv, st := newTestServer(t, map[string]*model.VehicleData{"golf": sampleVehicle()})

	body := bytes.NewBufferString(`{"date":"2025-06-01","miles":4000,"force":true}`)
	resp, err := http.Post(srv.URL+"/api/v1/vehicles/golf/readings", "application/json", body)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("forced add: want 200, got %d", resp.StatusCode)
	}
	data, _ := st.GetVehicle(context.Background(), "golf")
	if data.Readings["2025-06-01"] != 4000 {
		t.Fatalf("forced reading not stored: %+v", data.Readings)
	}
}

func TestAddReadingToMissingVehicle404(t *testing.T) {
	srv, _ := newTestServer(t, nil)

	body := bytes.NewBufferString(`{"date":"2025-06-01","miles":6000}`)
	resp, err := http.Post(srv.URL+"/api/v1/vehicles/ghost/readings", "application/json", body)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusNotFound {
		t.Fatalf("want 404, got %d", resp.StatusCode)
	}
}

func TestDeleteReading(t *testing.T) {
	srv, st := newTestServer(t, map[string]*model.VehicleData{"golf": sampleVehicle()})

	req, _ := http.NewRequest(http.MethodDelete, srv.URL+"/api/v1/vehicles/golf/readings/2025-01-01", nil)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("want 200, got %d", resp.StatusCode)
	}
	data, _ := st.GetVehicle(context.Background(), "golf")
	if _, ok := data.Readings["2025-01-01"]; ok {
		t.Fatal("reading not deleted")
	}
}

func TestDeleteMissingReading404(t *testing.T) {
	srv, _ := newTestServer(t, map[string]*model.VehicleData{"golf": sampleVehicle()})

	req, _ := http.NewRequest(http.MethodDelete, srv.URL+"/api/v1/vehicles/golf/readings/2099-01-01", nil)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusNotFound {
		t.Fatalf("want 404, got %d", resp.StatusCode)
	}
}

func TestCurrentGetSet(t *testing.T) {
	srv, _ := newTestServer(t, map[string]*model.VehicleData{"golf": sampleVehicle()})

	// Unset → "".
	resp, err := http.Get(srv.URL + "/api/v1/current")
	if err != nil {
		t.Fatal(err)
	}
	var got map[string]string
	json.NewDecoder(resp.Body).Decode(&got)
	resp.Body.Close()
	if got["current"] != "" {
		t.Fatalf("unset current: want \"\", got %q", got["current"])
	}

	// Set it.
	req, _ := http.NewRequest(http.MethodPut, srv.URL+"/api/v1/current", bytes.NewBufferString(`{"id":"golf"}`))
	req.Header.Set("Content-Type", "application/json")
	resp, err = http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("set current: want 200, got %d", resp.StatusCode)
	}

	// Read it back.
	resp, _ = http.Get(srv.URL + "/api/v1/current")
	json.NewDecoder(resp.Body).Decode(&got)
	resp.Body.Close()
	if got["current"] != "golf" {
		t.Fatalf("current: want golf, got %q", got["current"])
	}
}

func TestSetCurrentMissingVehicle404(t *testing.T) {
	srv, _ := newTestServer(t, nil)

	req, _ := http.NewRequest(http.MethodPut, srv.URL+"/api/v1/current", bytes.NewBufferString(`{"id":"ghost"}`))
	req.Header.Set("Content-Type", "application/json")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusNotFound {
		t.Fatalf("want 404, got %d", resp.StatusCode)
	}
}

func TestListVehiclesShape(t *testing.T) {
	srv, st := newTestServer(t, map[string]*model.VehicleData{"golf": sampleVehicle()})
	st.SetCurrent(context.Background(), "golf")

	resp, err := http.Get(srv.URL + "/api/v1/vehicles")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	var list []api.VehicleListItem
	if err := json.NewDecoder(resp.Body).Decode(&list); err != nil {
		t.Fatal(err)
	}
	if len(list) != 1 || list[0].ID != "golf" || !list[0].IsDefault {
		t.Fatalf("unexpected list: %+v", list)
	}
}

func TestFleetShape(t *testing.T) {
	srv, _ := newTestServer(t, nil)

	// Empty fleet must serialise Vehicles as [] (not null).
	resp, err := http.Get(srv.URL + "/api/v1/fleet")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	var fleet api.FleetResponse
	if err := json.NewDecoder(resp.Body).Decode(&fleet); err != nil {
		t.Fatal(err)
	}
	if fleet.Vehicles == nil {
		t.Fatal("empty fleet Vehicles should be [], got null")
	}
}

// The single-user router reports mode "single-user" at /api/v1/meta, with no
// auth required, so the SPA knows not to show a login flow.
func TestMetaReportsSingleUserMode(t *testing.T) {
	srv, _ := newTestServer(t, nil)

	resp, err := http.Get(srv.URL + "/api/v1/meta")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("want 200, got %d", resp.StatusCode)
	}
	var meta struct {
		Mode string `json:"mode"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&meta); err != nil {
		t.Fatal(err)
	}
	if meta.Mode != "single-user" {
		t.Fatalf("want mode single-user, got %q", meta.Mode)
	}
}

// The unversioned paths are gone: a clean break, since the only client is the
// co-updated SPA.
func TestUnversionedPathsAreGone(t *testing.T) {
	srv, _ := newTestServer(t, map[string]*model.VehicleData{"golf": sampleVehicle()})

	resp, err := http.Get(srv.URL + "/api/vehicles")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	// The SPA fallback serves index.html for unknown non-API paths, so the key
	// assertion is that it is not a successful JSON API response.
	if resp.StatusCode == http.StatusOK {
		if ct := resp.Header.Get("Content-Type"); ct == "application/json" {
			t.Fatal("unversioned /api/vehicles still serves the JSON API")
		}
	}
}

// --- CSV import ---

func importCSV(t *testing.T, srv *httptest.Server, id, query, body string) *http.Response {
	t.Helper()
	resp, err := http.Post(srv.URL+"/api/v1/vehicles/"+id+"/import"+query, "text/csv", bytes.NewBufferString(body))
	if err != nil {
		t.Fatal(err)
	}
	return resp
}

func TestImportCSVSuccess(t *testing.T) {
	srv, st := newTestServer(t, map[string]*model.VehicleData{"golf": sampleVehicle()})

	resp := importCSV(t, srv, "golf", "", "date,miles\n2025-02-01,5400\n2025-03-01,5900\n")
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		t.Fatalf("want 200, got %d: %s", resp.StatusCode, body)
	}
	var report struct {
		Status      string `json:"status"`
		Added       int    `json:"added"`
		Skipped     int    `json:"skipped"`
		Overwritten int    `json:"overwritten"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&report); err != nil {
		t.Fatal(err)
	}
	if report.Status != "imported" || report.Added != 2 || report.Skipped != 0 {
		t.Fatalf("unexpected report: %+v", report)
	}
	data, _ := st.GetVehicle(context.Background(), "golf")
	if data.Readings["2025-03-01"] != 5900 || len(data.Readings) != 3 {
		t.Fatalf("readings not persisted: %+v", data.Readings)
	}
}

func TestImportCSVMalformedRejectsAllWithLineDetails(t *testing.T) {
	srv, st := newTestServer(t, map[string]*model.VehicleData{"golf": sampleVehicle()})

	resp := importCSV(t, srv, "golf", "", "date,miles\n2025-02-01,5400\nnot-a-date,5500\n")
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("want 400, got %d", resp.StatusCode)
	}
	var body struct {
		Error struct {
			Code    string `json:"code"`
			Details []struct {
				Line    int    `json:"line"`
				Message string `json:"message"`
			} `json:"details"`
		} `json:"error"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		t.Fatal(err)
	}
	if body.Error.Code != "invalid_csv" || len(body.Error.Details) != 1 || body.Error.Details[0].Line != 3 {
		t.Fatalf("unexpected error body: %+v", body)
	}
	// All-or-nothing: the valid row must not have been persisted either.
	data, _ := st.GetVehicle(context.Background(), "golf")
	if _, ok := data.Readings["2025-02-01"]; ok {
		t.Fatal("valid row from a rejected file was persisted")
	}
}

func TestImportCSVMissingVehicle404(t *testing.T) {
	srv, _ := newTestServer(t, nil)

	resp := importCSV(t, srv, "ghost", "", "date,miles\n2025-02-01,5400\n")
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusNotFound {
		t.Fatalf("want 404, got %d", resp.StatusCode)
	}
}

func TestImportCSVSkipVsOverwrite(t *testing.T) {
	seed := sampleVehicle() // has 2025-01-01: 5000
	srv, st := newTestServer(t, map[string]*model.VehicleData{"golf": seed})

	// Default: existing wins, conflicting row skipped.
	resp := importCSV(t, srv, "golf", "", "date,miles\n2025-01-01,4900\n")
	resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("skip import: want 200, got %d", resp.StatusCode)
	}
	data, _ := st.GetVehicle(context.Background(), "golf")
	if data.Readings["2025-01-01"] != 5000 {
		t.Fatalf("existing reading clobbered without overwrite: %+v", data.Readings)
	}

	// overwrite=true replaces it.
	resp = importCSV(t, srv, "golf", "?overwrite=true", "date,miles\n2025-01-01,4900\n")
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("overwrite import: want 200, got %d", resp.StatusCode)
	}
	var report struct {
		Overwritten int `json:"overwritten"`
	}
	json.NewDecoder(resp.Body).Decode(&report)
	if report.Overwritten != 1 {
		t.Fatalf("want overwritten=1, got %+v", report)
	}
	data, _ = st.GetVehicle(context.Background(), "golf")
	if data.Readings["2025-01-01"] != 4900 {
		t.Fatalf("overwrite did not persist: %+v", data.Readings)
	}
}

func TestImportCSVMonotonicViolationNeedsForce(t *testing.T) {
	srv, st := newTestServer(t, map[string]*model.VehicleData{"golf": sampleVehicle()})

	// 2025-02-01 below the existing 2025-01-01 reading.
	csvBody := "date,miles\n2025-02-01,4000\n"
	resp := importCSV(t, srv, "golf", "", csvBody)
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("want 400, got %d", resp.StatusCode)
	}
	var body struct {
		Error struct {
			Code string `json:"code"`
		} `json:"error"`
	}
	json.NewDecoder(resp.Body).Decode(&body)
	if body.Error.Code != "not_monotonic" {
		t.Fatalf("want not_monotonic, got %+v", body)
	}
	data, _ := st.GetVehicle(context.Background(), "golf")
	if _, ok := data.Readings["2025-02-01"]; ok {
		t.Fatal("rejected import was persisted")
	}

	resp2 := importCSV(t, srv, "golf", "?force=true", csvBody)
	defer resp2.Body.Close()
	if resp2.StatusCode != http.StatusOK {
		t.Fatalf("forced import: want 200, got %d", resp2.StatusCode)
	}
	data, _ = st.GetVehicle(context.Background(), "golf")
	if data.Readings["2025-02-01"] != 4000 {
		t.Fatalf("forced import not persisted: %+v", data.Readings)
	}
}

// Round-trip guarantee: the export endpoint's body imported into a fresh
// vehicle reproduces identical readings.
func TestExportImportRoundTrip(t *testing.T) {
	source := sampleVehicle()
	source.Readings = map[string]int{
		"2025-01-01": 5000,
		"2025-03-15": 6210,
		"2025-06-30": 7345,
	}
	srv, st := newTestServer(t, map[string]*model.VehicleData{
		"golf":  source,
		"fresh": {Vehicle: "Fresh", Readings: map[string]int{}},
	})

	exportResp, err := http.Get(srv.URL + "/api/v1/vehicles/golf/export")
	if err != nil {
		t.Fatal(err)
	}
	exported, _ := io.ReadAll(exportResp.Body)
	exportResp.Body.Close()

	resp := importCSV(t, srv, "fresh", "", string(exported))
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		t.Fatalf("round-trip import: want 200, got %d: %s", resp.StatusCode, body)
	}

	got, _ := st.GetVehicle(context.Background(), "fresh")
	want, _ := st.GetVehicle(context.Background(), "golf")
	if !reflect.DeepEqual(got.Readings, want.Readings) {
		t.Fatalf("round-trip mismatch:\n got %v\nwant %v", got.Readings, want.Readings)
	}
}
