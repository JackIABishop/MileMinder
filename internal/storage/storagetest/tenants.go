package storagetest

import (
	"context"
	"errors"
	"testing"

	"github.com/jackiabishop/mileminder/internal/model"
	"github.com/jackiabishop/mileminder/internal/storage"
)

// RunTenantIsolation asserts a storage.Tenants keeps each user's data private:
// vehicles, readings, and the current pointer written through one user's Store
// are invisible through another user's. Every storage.Tenants implementation
// should pass it.
func RunTenantIsolation(t *testing.T, newTenants func(t *testing.T) storage.Tenants) {
	t.Helper()
	ctx := context.Background()

	t.Run("VehiclesAreOwnerPrivate", func(t *testing.T) {
		tn := newTenants(t)
		alice := tn.ForUser("alice")
		bob := tn.ForUser("bob")

		if err := alice.SaveVehicle(ctx, "golf", sampleVehicle("Golf")); err != nil {
			t.Fatalf("alice SaveVehicle: %v", err)
		}

		list, err := bob.ListVehicles(ctx)
		if err != nil {
			t.Fatalf("bob ListVehicles: %v", err)
		}
		if len(list) != 0 {
			t.Fatalf("bob sees alice's vehicles: %+v", list)
		}
		if _, err := bob.GetVehicle(ctx, "golf"); !errors.Is(err, storage.ErrNotFound) {
			t.Fatalf("bob GetVehicle(golf): want ErrNotFound, got %v", err)
		}
		if _, err := alice.GetVehicle(ctx, "golf"); err != nil {
			t.Fatalf("alice GetVehicle(golf) should still work: %v", err)
		}
	})

	t.Run("CurrentPointerIsOwnerPrivate", func(t *testing.T) {
		tn := newTenants(t)
		alice := tn.ForUser("alice")
		bob := tn.ForUser("bob")

		if err := alice.SaveVehicle(ctx, "golf", sampleVehicle("Golf")); err != nil {
			t.Fatalf("alice SaveVehicle: %v", err)
		}
		if err := alice.SetCurrent(ctx, "golf"); err != nil {
			t.Fatalf("alice SetCurrent: %v", err)
		}

		cur, err := bob.GetCurrent(ctx)
		if err != nil {
			t.Fatalf("bob GetCurrent: %v", err)
		}
		if cur != "" {
			t.Fatalf("bob sees alice's current pointer: %q", cur)
		}
	})

	t.Run("SettingsAreOwnerPrivate", func(t *testing.T) {
		tn := newTenants(t)
		alice := tn.ForUser("alice")
		bob := tn.ForUser("bob")

		if err := alice.SaveSettings(ctx, &model.Settings{Currency: "EUR", DistanceUnit: "mi"}); err != nil {
			t.Fatalf("alice SaveSettings: %v", err)
		}

		got, err := bob.GetSettings(ctx)
		if err != nil {
			t.Fatalf("bob GetSettings: %v", err)
		}
		if want := model.DefaultSettings(); *got != want {
			t.Fatalf("bob sees alice's settings: %+v", *got)
		}
	})

	t.Run("SameUserIDSharesData", func(t *testing.T) {
		tn := newTenants(t)
		if err := tn.ForUser("alice").SaveVehicle(ctx, "golf", sampleVehicle("Golf")); err != nil {
			t.Fatalf("SaveVehicle: %v", err)
		}
		// A fresh handle for the same user observes the same data.
		if _, err := tn.ForUser("alice").GetVehicle(ctx, "golf"); err != nil {
			t.Fatalf("second handle for same user should see data: %v", err)
		}
	})
}
