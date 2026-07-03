package alerts

import (
	"context"
	"errors"
	"testing"
	"time"
)

func TestFileStateStoreRoundTripAndPrune(t *testing.T) {
	ctx := context.Background()
	st := NewFileStateStore(t.TempDir())
	alertedAt := time.Date(2025, 4, 11, 12, 0, 0, 0, time.UTC)

	if _, err := st.GetState(ctx, "user-1", "golf"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("missing state: want ErrNotFound, got %v", err)
	}
	if err := st.PutState(ctx, VehicleAlertState{UserID: "user-1", VehicleID: "golf", Breached: true, LastAlertedAt: alertedAt}); err != nil {
		t.Fatalf("PutState: %v", err)
	}
	if err := st.PutState(ctx, VehicleAlertState{UserID: "user-1", VehicleID: "deleted", Breached: true}); err != nil {
		t.Fatalf("PutState deleted: %v", err)
	}
	got, err := st.GetState(ctx, "user-1", "golf")
	if err != nil {
		t.Fatalf("GetState: %v", err)
	}
	if !got.Breached || !got.LastAlertedAt.Equal(alertedAt) {
		t.Fatalf("state mismatch: %+v", got)
	}

	if err := st.PruneUserStates(ctx, "user-1", []string{"golf"}); err != nil {
		t.Fatalf("PruneUserStates: %v", err)
	}
	if _, err := st.GetState(ctx, "user-1", "deleted"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("pruned state: want ErrNotFound, got %v", err)
	}
}

func TestFilePrefsStoreRoundTrip(t *testing.T) {
	ctx := context.Background()
	st := NewFilePrefsStore(t.TempDir())

	if _, err := st.GetPrefs(ctx, "user-1"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("missing prefs: want ErrNotFound, got %v", err)
	}
	if err := st.PutPrefs(ctx, Prefs{UserID: "user-1", Enabled: true, Threshold: 90}); err != nil {
		t.Fatalf("PutPrefs: %v", err)
	}
	got, err := st.GetPrefs(ctx, "user-1")
	if err != nil {
		t.Fatalf("GetPrefs: %v", err)
	}
	if !got.Enabled || got.Threshold != 90 {
		t.Fatalf("prefs mismatch: %+v", got)
	}
	if err := st.PutPrefs(ctx, Prefs{UserID: "user-1", Enabled: false, Threshold: 75}); err != nil {
		t.Fatalf("PutPrefs update: %v", err)
	}
	got, err = st.GetPrefs(ctx, "user-1")
	if err != nil {
		t.Fatalf("GetPrefs updated: %v", err)
	}
	if got.Enabled || got.Threshold != 75 {
		t.Fatalf("updated prefs mismatch: %+v", got)
	}
}

func TestDefaultPrefs(t *testing.T) {
	got := DefaultPrefs("user-1")
	if got.UserID != "user-1" || !got.Enabled || got.Threshold != 100 {
		t.Fatalf("DefaultPrefs = %+v", got)
	}
}
