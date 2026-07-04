package api_test

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"testing"

	"github.com/jackiabishop/mileminder/internal/api"
	"github.com/jackiabishop/mileminder/internal/model"
)

func patchVehicle(t *testing.T, url, id, body string) *http.Response {
	t.Helper()
	req, err := http.NewRequest(http.MethodPatch, url+"/api/v1/vehicles/"+id, bytes.NewBufferString(body))
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { resp.Body.Close() })
	return resp
}

func TestCreateVehicleWithRegistration(t *testing.T) {
	srv, st := newTestServer(t, nil)

	resp, err := http.Post(srv.URL+"/api/v1/vehicles", "application/json", bytes.NewBufferString(`{
		"id":"golf",
		"vehicle":"Golf",
		"registration":"  AB12 CDE  ",
		"start_miles":5000
	}`))
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("create: status %d", resp.StatusCode)
	}

	data, err := st.GetVehicle(context.Background(), "golf")
	if err != nil {
		t.Fatal(err)
	}
	if data.Registration != "AB12 CDE" {
		t.Fatalf("registration not trimmed/persisted: %q", data.Registration)
	}
}

func TestPatchRegistrationOnPlainVehicle(t *testing.T) {
	srv, st := newTestServer(t, map[string]*model.VehicleData{
		"owned": {
			Vehicle:  "Owned Car",
			Readings: map[string]int{"2025-01-01": 5000},
		},
	})

	resp := patchVehicle(t, srv.URL, "owned", `{"registration":"XY99 ZZZ"}`)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("registration-only PATCH on plain vehicle: status %d", resp.StatusCode)
	}

	data, err := st.GetVehicle(context.Background(), "owned")
	if err != nil {
		t.Fatal(err)
	}
	if data.Registration != "XY99 ZZZ" {
		t.Fatalf("registration not persisted: %q", data.Registration)
	}
	if data.Plan != nil {
		t.Fatalf("plain vehicle grew a plan: %+v", data.Plan)
	}
}

func TestPatchIdentityLeavesPlanUntouched(t *testing.T) {
	srv, st := newTestServer(t, map[string]*model.VehicleData{"golf": sampleVehicle()})

	resp := patchVehicle(t, srv.URL, "golf", `{"vehicle":"Golf GTI","registration":"AB12 CDE"}`)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("identity PATCH: status %d", resp.StatusCode)
	}

	data, err := st.GetVehicle(context.Background(), "golf")
	if err != nil {
		t.Fatal(err)
	}
	if data.Vehicle != "Golf GTI" {
		t.Fatalf("rename not applied: %q", data.Vehicle)
	}
	if data.Registration != "AB12 CDE" {
		t.Fatalf("registration not applied: %q", data.Registration)
	}
	want := sampleVehicle().Plan
	if data.Plan == nil || *data.Plan != *want {
		t.Fatalf("plan changed by identity PATCH: got %+v want %+v", data.Plan, want)
	}
}

func TestRegistrationInListStatusAndProfile(t *testing.T) {
	v := sampleVehicle()
	v.Registration = "AB12 CDE"
	srv, _ := newTestServer(t, map[string]*model.VehicleData{"golf": v})

	// List
	resp, err := http.Get(srv.URL + "/api/v1/vehicles")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	var list []api.VehicleListItem
	if err := json.NewDecoder(resp.Body).Decode(&list); err != nil {
		t.Fatal(err)
	}
	if len(list) != 1 || list[0].Registration != "AB12 CDE" {
		t.Fatalf("list missing registration: %+v", list)
	}

	// Status
	resp2, err := http.Get(srv.URL + "/api/v1/vehicles/golf")
	if err != nil {
		t.Fatal(err)
	}
	defer resp2.Body.Close()
	var status api.VehicleStatus
	if err := json.NewDecoder(resp2.Body).Decode(&status); err != nil {
		t.Fatal(err)
	}
	if status.Registration != "AB12 CDE" {
		t.Fatalf("status missing registration: %+v", status)
	}

	// Profile export
	resp3, err := http.Get(srv.URL + "/api/v1/vehicles/golf/profile")
	if err != nil {
		t.Fatal(err)
	}
	defer resp3.Body.Close()
	var profile api.VehicleProfile
	if err := json.NewDecoder(resp3.Body).Decode(&profile); err != nil {
		t.Fatal(err)
	}
	if profile.Registration != "AB12 CDE" {
		t.Fatalf("profile missing registration: %+v", profile)
	}
}
