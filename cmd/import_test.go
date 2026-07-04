package cmd

import (
	"context"
	"reflect"
	"strings"
	"testing"

	"github.com/jackiabishop/mileminder/internal/model"
	"github.com/jackiabishop/mileminder/internal/readings"
	"github.com/jackiabishop/mileminder/internal/storage"
)

func seedStore(t *testing.T, existing map[string]int) storage.Store {
	t.Helper()
	st := storage.NewMemory()
	data := &model.VehicleData{Vehicle: "Golf", Readings: existing}
	if err := st.SaveVehicle(context.Background(), "golf", data); err != nil {
		t.Fatal(err)
	}
	return st
}

func TestRunImportAddsReadings(t *testing.T) {
	st := seedStore(t, map[string]int{"2025-01-01": 5000})

	csv := "date,miles\n2025-02-01,5400\n2025-03-01,5900\n"
	report, err := runImport(context.Background(), st, "golf", strings.NewReader(csv), false, false)
	if err != nil {
		t.Fatalf("runImport: %v", err)
	}
	if report != (readings.Report{Added: 2}) {
		t.Fatalf("report = %+v", report)
	}
	data, _ := st.GetVehicle(context.Background(), "golf")
	want := map[string]int{"2025-01-01": 5000, "2025-02-01": 5400, "2025-03-01": 5900}
	if !reflect.DeepEqual(data.Readings, want) {
		t.Fatalf("readings = %v, want %v", data.Readings, want)
	}
}

func TestRunImportAllOrNothingOnBadRows(t *testing.T) {
	st := seedStore(t, map[string]int{"2025-01-01": 5000})

	csv := "date,miles\n2025-02-01,5400\nbogus,5500\n"
	_, err := runImport(context.Background(), st, "golf", strings.NewReader(csv), false, false)
	if err == nil {
		t.Fatal("want error for invalid row")
	}
	if !strings.Contains(err.Error(), "line 3") {
		t.Fatalf("error not line-numbered: %v", err)
	}
	data, _ := st.GetVehicle(context.Background(), "golf")
	if _, ok := data.Readings["2025-02-01"]; ok {
		t.Fatal("valid row from a rejected file was persisted")
	}
}

func TestRunImportSkipAndOverwrite(t *testing.T) {
	csv := "date,miles\n2025-01-01,4900\n"

	st := seedStore(t, map[string]int{"2025-01-01": 5000})
	report, err := runImport(context.Background(), st, "golf", strings.NewReader(csv), false, false)
	if err != nil {
		t.Fatalf("skip import: %v", err)
	}
	if report.Skipped != 1 || report.Added != 0 {
		t.Fatalf("report = %+v", report)
	}
	data, _ := st.GetVehicle(context.Background(), "golf")
	if data.Readings["2025-01-01"] != 5000 {
		t.Fatal("existing reading clobbered without overwrite")
	}

	report, err = runImport(context.Background(), st, "golf", strings.NewReader(csv), true, false)
	if err != nil {
		t.Fatalf("overwrite import: %v", err)
	}
	if report.Overwritten != 1 {
		t.Fatalf("report = %+v", report)
	}
	data, _ = st.GetVehicle(context.Background(), "golf")
	if data.Readings["2025-01-01"] != 4900 {
		t.Fatal("overwrite not persisted")
	}
}

func TestRunImportMonotonicNeedsForce(t *testing.T) {
	st := seedStore(t, map[string]int{"2025-01-01": 5000})

	csv := "date,miles\n2025-02-01,4000\n"
	_, err := runImport(context.Background(), st, "golf", strings.NewReader(csv), false, false)
	if err == nil || !strings.Contains(err.Error(), "--force") {
		t.Fatalf("want monotonic error mentioning --force, got %v", err)
	}
	data, _ := st.GetVehicle(context.Background(), "golf")
	if _, ok := data.Readings["2025-02-01"]; ok {
		t.Fatal("rejected import was persisted")
	}

	report, err := runImport(context.Background(), st, "golf", strings.NewReader(csv), false, true)
	if err != nil {
		t.Fatalf("forced import: %v", err)
	}
	if report.Added != 1 {
		t.Fatalf("report = %+v", report)
	}
}

func TestRunImportMissingVehicle(t *testing.T) {
	st := storage.NewMemory()
	_, err := runImport(context.Background(), st, "ghost", strings.NewReader("date,miles\n"), false, false)
	if err == nil {
		t.Fatal("want error for missing vehicle")
	}
}
