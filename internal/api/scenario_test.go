package api_test

import (
	"bytes"
	"context"
	"encoding/json"
	"math"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"reflect"
	"testing"
	"time"

	"github.com/jackiabishop/mileminder/internal/api"
	"github.com/jackiabishop/mileminder/internal/model"
	"github.com/jackiabishop/mileminder/internal/storage/yamlstore"
)

// scenarioResp mirrors calc.Scenario for decoding the endpoint's JSON.
type scenarioResp struct {
	ExtraMiles        float64           `json:"extra_miles"`
	ByDate            string            `json:"by_date"`
	BaselineMiles     float64           `json:"baseline_miles"`
	HypotheticalMiles float64           `json:"hypothetical_miles"`
	Status            api.VehicleStatus `json:"status"`
}

// scenarioVehicle builds a policy vehicle relative to now so the endpoint
// (which uses real time.Now()) sees a valid past-history + future-date setup
// regardless of the wall clock the test runs at.
func scenarioVehicle() *model.VehicleData {
	now := time.Now()
	return &model.VehicleData{
		Vehicle: "Golf",
		Plan: &model.Plan{
			Start:           now.AddDate(-1, 0, 0),
			End:             now.AddDate(2, 0, 0),
			AnnualAllowance: 10000,
			StartMiles:      5000,
			ExcessRate:      10,
		},
		Readings: map[string]int{
			now.AddDate(-1, 0, 0).Format("2006-01-02"): 5000,
			now.AddDate(0, 0, -30).Format("2006-01-02"): 8000,
		},
	}
}

func postScenario(t *testing.T, srv *httptest.Server, id, body string) *http.Response {
	t.Helper()
	resp, err := http.Post(srv.URL+"/api/v1/vehicles/"+id+"/scenario", "application/json", bytes.NewBufferString(body))
	if err != nil {
		t.Fatal(err)
	}
	return resp
}

func TestScenarioHappyPath(t *testing.T) {
	srv, st := newTestServer(t, map[string]*model.VehicleData{"golf": scenarioVehicle()})

	byDate := time.Now().AddDate(0, 1, 0).Format("2006-01-02")
	resp := postScenario(t, srv, "golf", `{"extra_miles":600,"by_date":"`+byDate+`"}`)
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("want 200, got %d", resp.StatusCode)
	}

	var sc scenarioResp
	if err := json.NewDecoder(resp.Body).Decode(&sc); err != nil {
		t.Fatal(err)
	}
	if math.Abs(sc.HypotheticalMiles-(sc.BaselineMiles+600)) > 1e-6 {
		t.Errorf("hypothetical_miles %v != baseline_miles %v + 600", sc.HypotheticalMiles, sc.BaselineMiles)
	}
	if sc.Status.LatestDate != byDate {
		t.Errorf("status.latest_date = %q, want %q", sc.Status.LatestDate, byDate)
	}
	if !sc.Status.HasPlan {
		t.Error("status.has_plan = false, want true")
	}
	if sc.ByDate != byDate || sc.ExtraMiles != 600 {
		t.Errorf("echoed inputs = %q/%v, want %q/600", sc.ByDate, sc.ExtraMiles, byDate)
	}

	// The store must be untouched: readings identical to the seed.
	got, err := st.GetVehicle(context.Background(), "golf")
	if err != nil {
		t.Fatal(err)
	}
	want := scenarioVehicle().Readings
	if !reflect.DeepEqual(got.Readings, want) {
		t.Errorf("scenario mutated stored readings: got %v, want %v", got.Readings, want)
	}
}

func TestScenarioValidation(t *testing.T) {
	seed := map[string]*model.VehicleData{
		"golf":  scenarioVehicle(),
		"plain": {Vehicle: "Owned", Readings: map[string]int{"2025-01-01": 10000}},
	}
	srv, _ := newTestServer(t, seed)

	future := time.Now().AddDate(0, 1, 0).Format("2006-01-02")
	afterEnd := time.Now().AddDate(3, 0, 0).Format("2006-01-02")

	cases := []struct {
		name     string
		id       string
		body     string
		wantCode string // "" means expect a 404 (plain error, no envelope)
		wantHTTP int
	}{
		{"missing by_date", "golf", `{"extra_miles":600}`, "invalid_by_date", http.StatusBadRequest},
		{"malformed by_date", "golf", `{"extra_miles":600,"by_date":"nope"}`, "invalid_by_date", http.StatusBadRequest},
		{"missing extra_miles", "golf", `{"by_date":"` + future + `"}`, "missing_extra_miles", http.StatusBadRequest},
		{"negative extra_miles", "golf", `{"extra_miles":-5,"by_date":"` + future + `"}`, "invalid_extra_miles", http.StatusBadRequest},
		{"by_date in past", "golf", `{"extra_miles":600,"by_date":"2020-01-01"}`, "by_date_not_future", http.StatusBadRequest},
		{"by_date after plan end", "golf", `{"extra_miles":600,"by_date":"` + afterEnd + `"}`, "by_date_after_plan_end", http.StatusBadRequest},
		{"plan-less vehicle", "plain", `{"extra_miles":600,"by_date":"` + future + `"}`, "vehicle_has_no_plan", http.StatusBadRequest},
		{"unknown vehicle", "ghost", `{"extra_miles":600,"by_date":"` + future + `"}`, "", http.StatusNotFound},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			resp := postScenario(t, srv, tc.id, tc.body)
			defer resp.Body.Close()
			if resp.StatusCode != tc.wantHTTP {
				t.Fatalf("status = %d, want %d", resp.StatusCode, tc.wantHTTP)
			}
			if tc.wantCode == "" {
				return
			}
			var env struct {
				Error struct {
					Code string `json:"code"`
				} `json:"error"`
			}
			if err := json.NewDecoder(resp.Body).Decode(&env); err != nil {
				t.Fatal(err)
			}
			if env.Error.Code != tc.wantCode {
				t.Errorf("error code = %q, want %q", env.Error.Code, tc.wantCode)
			}
		})
	}
}

// TestScenarioDoesNotWriteYAMLStore is the core safety property of issue #9: a
// scenario call must never touch persisted storage. It asserts the vehicle's
// YAML file is byte-identical before and after a successful scenario, and that
// no stray files (e.g. atomic temp files) are left behind.
func TestScenarioDoesNotWriteYAMLStore(t *testing.T) {
	dir := t.TempDir()
	st := yamlstore.New(dir)
	if err := st.SaveVehicle(context.Background(), "golf", scenarioVehicle()); err != nil {
		t.Fatal(err)
	}
	srv := httptest.NewServer(api.NewRouter(st, ""))
	t.Cleanup(srv.Close)

	path := filepath.Join(dir, "golf.yml")
	before, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	beforeEntries := readDirNames(t, dir)

	byDate := time.Now().AddDate(0, 1, 0).Format("2006-01-02")
	resp := postScenario(t, srv, "golf", `{"extra_miles":600,"by_date":"`+byDate+`"}`)
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("want 200, got %d", resp.StatusCode)
	}

	after, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(before, after) {
		t.Error("scenario call modified the vehicle's YAML on disk")
	}
	if got := readDirNames(t, dir); !reflect.DeepEqual(got, beforeEntries) {
		t.Errorf("scenario call changed data-dir contents: got %v, want %v", got, beforeEntries)
	}
}

func readDirNames(t *testing.T, dir string) []string {
	t.Helper()
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	names := make([]string, 0, len(entries))
	for _, e := range entries {
		names = append(names, e.Name())
	}
	return names
}
