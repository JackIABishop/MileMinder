package api_test

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
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
		Plan: model.Plan{
			Start:           time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC),
			End:             time.Date(2028, 1, 1, 0, 0, 0, 0, time.UTC),
			AnnualAllowance: 10000,
			StartMiles:      5000,
		},
		Readings: map[string]int{"2025-01-01": 5000},
	}
}

func TestGetMissingVehicleReturns404(t *testing.T) {
	srv, _ := newTestServer(t, nil)

	resp, err := http.Get(srv.URL + "/api/vehicles/ghost")
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

	resp, err := http.Get(srv.URL + "/api/vehicles/golf")
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
	resp, err := http.Post(srv.URL+"/api/vehicles/golf/readings", "application/json", body)
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
	resp, err := http.Post(srv.URL+"/api/vehicles/golf/readings", "application/json", body)
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
	resp, err := http.Post(srv.URL+"/api/vehicles/ghost/readings", "application/json", body)
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

	req, _ := http.NewRequest(http.MethodDelete, srv.URL+"/api/vehicles/golf/readings/2025-01-01", nil)
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

	req, _ := http.NewRequest(http.MethodDelete, srv.URL+"/api/vehicles/golf/readings/2099-01-01", nil)
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
	resp, err := http.Get(srv.URL + "/api/current")
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
	req, _ := http.NewRequest(http.MethodPut, srv.URL+"/api/current", bytes.NewBufferString(`{"id":"golf"}`))
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
	resp, _ = http.Get(srv.URL + "/api/current")
	json.NewDecoder(resp.Body).Decode(&got)
	resp.Body.Close()
	if got["current"] != "golf" {
		t.Fatalf("current: want golf, got %q", got["current"])
	}
}

func TestSetCurrentMissingVehicle404(t *testing.T) {
	srv, _ := newTestServer(t, nil)

	req, _ := http.NewRequest(http.MethodPut, srv.URL+"/api/current", bytes.NewBufferString(`{"id":"ghost"}`))
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

	resp, err := http.Get(srv.URL + "/api/vehicles")
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
	resp, err := http.Get(srv.URL + "/api/fleet")
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
