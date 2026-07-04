// Package storagetest provides a shared conformance suite so every storage.Store
// implementation is held to the same observable behaviour.
package storagetest

import (
	"context"
	"errors"
	"reflect"
	"testing"
	"time"

	"github.com/jackiabishop/mileminder/internal/model"
	"github.com/jackiabishop/mileminder/internal/storage"
)

// sampleVehicle returns a small, deterministic VehicleData for tests.
func sampleVehicle(name string) *model.VehicleData {
	return &model.VehicleData{
		Vehicle: name,
		Plan: &model.Plan{
			Start:           time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC),
			End:             time.Date(2028, 1, 1, 0, 0, 0, 0, time.UTC),
			AnnualAllowance: 10000,
			StartMiles:      5000,
		},
		Readings: map[string]int{"2025-01-01": 5000},
	}
}

// RunConformance exercises the full storage.Store contract against a fresh store
// produced by newStore. Every storage.Store implementation should pass it.
func RunConformance(t *testing.T, newStore func(t *testing.T) storage.Store) {
	t.Helper()
	ctx := context.Background()

	t.Run("GetMissingVehicle", func(t *testing.T) {
		st := newStore(t)
		if _, err := st.GetVehicle(ctx, "nope"); !errors.Is(err, storage.ErrNotFound) {
			t.Fatalf("GetVehicle missing: want ErrNotFound, got %v", err)
		}
	})

	t.Run("SaveThenGetRoundTrip", func(t *testing.T) {
		st := newStore(t)
		want := sampleVehicle("Golf")
		if err := st.SaveVehicle(ctx, "golf", want); err != nil {
			t.Fatalf("SaveVehicle: %v", err)
		}
		got, err := st.GetVehicle(ctx, "golf")
		if err != nil {
			t.Fatalf("GetVehicle: %v", err)
		}
		if got.Vehicle != want.Vehicle || !reflect.DeepEqual(got.Plan, want.Plan) {
			t.Fatalf("round trip mismatch: got %+v want %+v", got, want)
		}
		if got.Readings["2025-01-01"] != 5000 {
			t.Fatalf("reading lost in round trip: %+v", got.Readings)
		}
	})

	t.Run("PlainVehicleRoundTrip", func(t *testing.T) {
		st := newStore(t)
		want := &model.VehicleData{
			Vehicle:  "Owned Car",
			Plan:     nil,
			Readings: map[string]int{"2025-01-01": 5000, "2025-06-01": 6100},
		}
		if err := st.SaveVehicle(ctx, "owned", want); err != nil {
			t.Fatalf("SaveVehicle: %v", err)
		}
		got, err := st.GetVehicle(ctx, "owned")
		if err != nil {
			t.Fatalf("GetVehicle: %v", err)
		}
		if got.Plan != nil {
			t.Fatalf("plain vehicle got non-nil plan: %+v", got.Plan)
		}
		if got.Vehicle != want.Vehicle || !reflect.DeepEqual(got.Readings, want.Readings) {
			t.Fatalf("plain round trip mismatch: got %+v want %+v", got, want)
		}
	})

	t.Run("SaveVehicleUpserts", func(t *testing.T) {
		st := newStore(t)
		if err := st.SaveVehicle(ctx, "golf", sampleVehicle("Golf")); err != nil {
			t.Fatalf("SaveVehicle: %v", err)
		}
		updated := sampleVehicle("Golf GTI")
		if err := st.SaveVehicle(ctx, "golf", updated); err != nil {
			t.Fatalf("SaveVehicle (upsert): %v", err)
		}
		got, err := st.GetVehicle(ctx, "golf")
		if err != nil {
			t.Fatalf("GetVehicle: %v", err)
		}
		if got.Vehicle != "Golf GTI" {
			t.Fatalf("upsert did not replace: got %q", got.Vehicle)
		}
	})

	t.Run("GetReturnsIndependentCopy", func(t *testing.T) {
		st := newStore(t)
		if err := st.SaveVehicle(ctx, "golf", sampleVehicle("Golf")); err != nil {
			t.Fatalf("SaveVehicle: %v", err)
		}
		got, err := st.GetVehicle(ctx, "golf")
		if err != nil {
			t.Fatalf("GetVehicle: %v", err)
		}
		got.Readings["2025-06-01"] = 9999 // must not leak back into the store
		got.Plan.ExcessRate = 99          // must not alias the stored plan either
		reread, err := st.GetVehicle(ctx, "golf")
		if err != nil {
			t.Fatalf("GetVehicle: %v", err)
		}
		if _, ok := reread.Readings["2025-06-01"]; ok {
			t.Fatal("mutating a returned vehicle leaked into the store")
		}
		if reread.Plan.ExcessRate == 99 {
			t.Fatal("mutating a returned plan leaked into the store")
		}
	})

	t.Run("DeleteVehicle", func(t *testing.T) {
		st := newStore(t)
		if err := st.SaveVehicle(ctx, "golf", sampleVehicle("Golf")); err != nil {
			t.Fatalf("SaveVehicle: %v", err)
		}
		if err := st.DeleteVehicle(ctx, "golf"); err != nil {
			t.Fatalf("DeleteVehicle: %v", err)
		}
		if _, err := st.GetVehicle(ctx, "golf"); !errors.Is(err, storage.ErrNotFound) {
			t.Fatalf("after delete: want ErrNotFound, got %v", err)
		}
		if err := st.DeleteVehicle(ctx, "golf"); !errors.Is(err, storage.ErrNotFound) {
			t.Fatalf("delete missing: want ErrNotFound, got %v", err)
		}
	})

	t.Run("ListVehicles", func(t *testing.T) {
		st := newStore(t)
		if got, err := st.ListVehicles(ctx); err != nil || len(got) != 0 {
			t.Fatalf("empty list: got %v err %v", got, err)
		}
		if err := st.SaveVehicle(ctx, "golf", sampleVehicle("Golf")); err != nil {
			t.Fatalf("SaveVehicle: %v", err)
		}
		if err := st.SaveVehicle(ctx, "polo", sampleVehicle("Polo")); err != nil {
			t.Fatalf("SaveVehicle: %v", err)
		}
		got, err := st.ListVehicles(ctx)
		if err != nil {
			t.Fatalf("ListVehicles: %v", err)
		}
		if len(got) != 2 {
			t.Fatalf("want 2 vehicles, got %d", len(got))
		}
		ids := map[string]bool{}
		for _, r := range got {
			ids[r.ID] = true
			if r.Data == nil {
				t.Fatalf("record %q has nil Data", r.ID)
			}
		}
		if !ids["golf"] || !ids["polo"] {
			t.Fatalf("missing ids in list: %v", ids)
		}
	})

	t.Run("PutReading", func(t *testing.T) {
		st := newStore(t)
		if err := st.PutReading(ctx, "golf", "2025-06-01", 6000); !errors.Is(err, storage.ErrNotFound) {
			t.Fatalf("PutReading missing vehicle: want ErrNotFound, got %v", err)
		}
		if err := st.SaveVehicle(ctx, "golf", sampleVehicle("Golf")); err != nil {
			t.Fatalf("SaveVehicle: %v", err)
		}
		if err := st.PutReading(ctx, "golf", "2025-06-01", 6000); err != nil {
			t.Fatalf("PutReading: %v", err)
		}
		got, err := st.GetVehicle(ctx, "golf")
		if err != nil {
			t.Fatalf("GetVehicle: %v", err)
		}
		if got.Readings["2025-06-01"] != 6000 {
			t.Fatalf("reading not stored: %+v", got.Readings)
		}
		// Upsert same date.
		if err := st.PutReading(ctx, "golf", "2025-06-01", 6100); err != nil {
			t.Fatalf("PutReading upsert: %v", err)
		}
		got, _ = st.GetVehicle(ctx, "golf")
		if got.Readings["2025-06-01"] != 6100 {
			t.Fatalf("reading not upserted: %+v", got.Readings)
		}
	})

	t.Run("DeleteReading", func(t *testing.T) {
		st := newStore(t)
		if err := st.DeleteReading(ctx, "golf", "2025-01-01"); !errors.Is(err, storage.ErrNotFound) {
			t.Fatalf("DeleteReading missing vehicle: want ErrNotFound, got %v", err)
		}
		if err := st.SaveVehicle(ctx, "golf", sampleVehicle("Golf")); err != nil {
			t.Fatalf("SaveVehicle: %v", err)
		}
		if err := st.DeleteReading(ctx, "golf", "2099-01-01"); !errors.Is(err, storage.ErrNotFound) {
			t.Fatalf("DeleteReading missing reading: want ErrNotFound, got %v", err)
		}
		if err := st.DeleteReading(ctx, "golf", "2025-01-01"); err != nil {
			t.Fatalf("DeleteReading: %v", err)
		}
		got, _ := st.GetVehicle(ctx, "golf")
		if _, ok := got.Readings["2025-01-01"]; ok {
			t.Fatal("reading not deleted")
		}
	})

	t.Run("SettingsDefaultWhenAbsent", func(t *testing.T) {
		st := newStore(t)
		got, err := st.GetSettings(ctx)
		if err != nil {
			t.Fatalf("GetSettings on fresh store: %v", err)
		}
		if want := model.DefaultSettings(); *got != want {
			t.Fatalf("fresh settings: want %+v, got %+v", want, *got)
		}
	})

	t.Run("SettingsRoundTrip", func(t *testing.T) {
		st := newStore(t)
		want := &model.Settings{Currency: "EUR", DistanceUnit: "mi"}
		if err := st.SaveSettings(ctx, want); err != nil {
			t.Fatalf("SaveSettings: %v", err)
		}
		got, err := st.GetSettings(ctx)
		if err != nil {
			t.Fatalf("GetSettings: %v", err)
		}
		if *got != *want {
			t.Fatalf("settings round trip: want %+v, got %+v", *want, *got)
		}
	})

	t.Run("SettingsReturnsIndependentCopy", func(t *testing.T) {
		st := newStore(t)
		if err := st.SaveSettings(ctx, &model.Settings{Currency: "EUR", DistanceUnit: "mi"}); err != nil {
			t.Fatalf("SaveSettings: %v", err)
		}
		got, err := st.GetSettings(ctx)
		if err != nil {
			t.Fatalf("GetSettings: %v", err)
		}
		got.Currency = "XXX" // must not leak back into the store
		reread, err := st.GetSettings(ctx)
		if err != nil {
			t.Fatalf("GetSettings: %v", err)
		}
		if reread.Currency != "EUR" {
			t.Fatal("mutating returned settings leaked into the store")
		}
	})

	t.Run("CurrentPointer", func(t *testing.T) {
		st := newStore(t)
		if cur, err := st.GetCurrent(ctx); err != nil || cur != "" {
			t.Fatalf("unset current: want \"\", got %q err %v", cur, err)
		}
		if err := st.SetCurrent(ctx, "ghost"); !errors.Is(err, storage.ErrNotFound) {
			t.Fatalf("SetCurrent missing vehicle: want ErrNotFound, got %v", err)
		}
		if err := st.SaveVehicle(ctx, "golf", sampleVehicle("Golf")); err != nil {
			t.Fatalf("SaveVehicle: %v", err)
		}
		if err := st.SetCurrent(ctx, "golf"); err != nil {
			t.Fatalf("SetCurrent: %v", err)
		}
		if cur, err := st.GetCurrent(ctx); err != nil || cur != "golf" {
			t.Fatalf("current: want \"golf\", got %q err %v", cur, err)
		}
	})
}
