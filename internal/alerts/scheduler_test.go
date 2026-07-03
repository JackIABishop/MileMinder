package alerts

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/jackiabishop/mileminder/internal/auth"
	"github.com/jackiabishop/mileminder/internal/model"
	"github.com/jackiabishop/mileminder/internal/notify"
	"github.com/jackiabishop/mileminder/internal/storage"
)

func alertDate(s string) time.Time {
	t, err := time.Parse("2006-01-02", s)
	if err != nil {
		panic(err)
	}
	return t
}

func policyVehicle(miles int) *model.VehicleData {
	return &model.VehicleData{
		Vehicle: "Golf",
		Plan: &model.Plan{
			Start:           alertDate("2025-01-01"),
			End:             alertDate("2026-01-01"),
			AnnualAllowance: 10000,
			StartMiles:      0,
		},
		Readings: map[string]int{
			"2025-01-01": 0,
			"2025-04-11": miles,
		},
	}
}

type schedulerFixture struct {
	users   *auth.MemoryUserStore
	tenants *storage.MemoryTenants
	state   *MemoryStateStore
	prefs   *MemoryPrefsStore
	fake    *notify.Fake
	user    *auth.User
	sched   *Scheduler
}

func newSchedulerFixture(t *testing.T) schedulerFixture {
	t.Helper()
	ctx := context.Background()
	users := auth.NewMemoryUserStore()
	user, err := users.CreateUser(ctx, "alice@example.com", "hash")
	if err != nil {
		t.Fatalf("CreateUser: %v", err)
	}
	f := schedulerFixture{
		users:   users,
		tenants: storage.NewMemoryTenants(),
		state:   NewMemoryStateStore(),
		prefs:   NewMemoryPrefsStore(),
		fake:    notify.NewFake(),
		user:    user,
	}
	f.sched = &Scheduler{
		Users:   f.users,
		Tenants: f.tenants,
		State:   f.state,
		Prefs:   f.prefs,
		Channel: f.fake,
		Now:     func() time.Time { return alertDate("2025-04-11") },
		BaseURL: "https://app.example.com",
	}
	return f
}

func TestRunOnceSilentFirstRunBaseline(t *testing.T) {
	ctx := context.Background()
	f := newSchedulerFixture(t)
	if err := f.tenants.ForUser(f.user.ID).SaveVehicle(ctx, "golf", policyVehicle(5000)); err != nil {
		t.Fatalf("SaveVehicle: %v", err)
	}

	f.sched.RunOnce(ctx)

	if got := len(f.fake.Deliveries()); got != 0 {
		t.Fatalf("deliveries = %d, want 0", got)
	}
	st, err := f.state.GetState(ctx, f.user.ID, "golf")
	if err != nil {
		t.Fatalf("GetState: %v", err)
	}
	if !st.Breached || !st.LastAlertedAt.IsZero() {
		t.Fatalf("baseline state = %+v, want breached with no alert timestamp", st)
	}
}

func TestRunOnceCrossingSendsOnce(t *testing.T) {
	ctx := context.Background()
	f := newSchedulerFixture(t)
	if err := f.tenants.ForUser(f.user.ID).SaveVehicle(ctx, "golf", policyVehicle(5000)); err != nil {
		t.Fatalf("SaveVehicle: %v", err)
	}
	if err := f.state.PutState(ctx, VehicleAlertState{UserID: f.user.ID, VehicleID: "golf", Breached: false}); err != nil {
		t.Fatalf("PutState: %v", err)
	}

	f.sched.RunOnce(ctx)
	f.sched.RunOnce(ctx)

	deliveries := f.fake.Deliveries()
	if len(deliveries) != 1 {
		t.Fatalf("deliveries = %d, want 1", len(deliveries))
	}
	if deliveries[0].To.Email != "alice@example.com" {
		t.Fatalf("recipient = %q", deliveries[0].To.Email)
	}
	body := deliveries[0].Message.Body
	if !strings.Contains(body, "Golf") || !strings.Contains(body, "Manage alerts in Settings") {
		t.Fatalf("message body missing expected content:\n%s", body)
	}
	st, err := f.state.GetState(ctx, f.user.ID, "golf")
	if err != nil {
		t.Fatalf("GetState: %v", err)
	}
	if !st.Breached || st.LastAlertedAt.IsZero() {
		t.Fatalf("state after send = %+v, want breached with timestamp", st)
	}
}

func TestRunOnceClearAndRecrossSendsAgain(t *testing.T) {
	ctx := context.Background()
	f := newSchedulerFixture(t)
	store := f.tenants.ForUser(f.user.ID)
	if err := store.SaveVehicle(ctx, "golf", policyVehicle(5000)); err != nil {
		t.Fatalf("SaveVehicle: %v", err)
	}
	if err := f.state.PutState(ctx, VehicleAlertState{UserID: f.user.ID, VehicleID: "golf", Breached: false}); err != nil {
		t.Fatalf("PutState: %v", err)
	}

	f.sched.RunOnce(ctx)
	if err := store.SaveVehicle(ctx, "golf", policyVehicle(1000)); err != nil {
		t.Fatalf("SaveVehicle clear: %v", err)
	}
	f.sched.RunOnce(ctx)
	if err := store.SaveVehicle(ctx, "golf", policyVehicle(5000)); err != nil {
		t.Fatalf("SaveVehicle recross: %v", err)
	}
	f.sched.RunOnce(ctx)

	if got := len(f.fake.Deliveries()); got != 2 {
		t.Fatalf("deliveries = %d, want 2", got)
	}
}

func TestRunOnceSendFailureRetries(t *testing.T) {
	ctx := context.Background()
	f := newSchedulerFixture(t)
	if err := f.tenants.ForUser(f.user.ID).SaveVehicle(ctx, "golf", policyVehicle(5000)); err != nil {
		t.Fatalf("SaveVehicle: %v", err)
	}
	if err := f.state.PutState(ctx, VehicleAlertState{UserID: f.user.ID, VehicleID: "golf", Breached: false}); err != nil {
		t.Fatalf("PutState: %v", err)
	}
	f.fake.SetError(errors.New("temporary"))

	f.sched.RunOnce(ctx)
	st, err := f.state.GetState(ctx, f.user.ID, "golf")
	if err != nil {
		t.Fatalf("GetState: %v", err)
	}
	if st.Breached {
		t.Fatalf("state persisted after failed send: %+v", st)
	}

	f.fake.SetError(nil)
	f.sched.RunOnce(ctx)
	if got := len(f.fake.Deliveries()); got != 1 {
		t.Fatalf("deliveries after retry = %d, want 1", got)
	}
}

func TestRunOnceOptedOutSkips(t *testing.T) {
	ctx := context.Background()
	f := newSchedulerFixture(t)
	if err := f.tenants.ForUser(f.user.ID).SaveVehicle(ctx, "golf", policyVehicle(5000)); err != nil {
		t.Fatalf("SaveVehicle: %v", err)
	}
	if err := f.prefs.PutPrefs(ctx, Prefs{UserID: f.user.ID, Enabled: false, Threshold: 100}); err != nil {
		t.Fatalf("PutPrefs: %v", err)
	}

	f.sched.RunOnce(ctx)

	if got := len(f.fake.Deliveries()); got != 0 {
		t.Fatalf("deliveries = %d, want 0", got)
	}
	if _, err := f.state.GetState(ctx, f.user.ID, "golf"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("state for opted-out user: want ErrNotFound, got %v", err)
	}
}

func TestRunOnceSkipsPlainVehicles(t *testing.T) {
	ctx := context.Background()
	f := newSchedulerFixture(t)
	plain := &model.VehicleData{Vehicle: "Owned", Readings: map[string]int{"2025-01-01": 10000, "2025-04-11": 20000}}
	if err := f.tenants.ForUser(f.user.ID).SaveVehicle(ctx, "owned", plain); err != nil {
		t.Fatalf("SaveVehicle: %v", err)
	}

	f.sched.RunOnce(ctx)

	if got := len(f.fake.Deliveries()); got != 0 {
		t.Fatalf("deliveries = %d, want 0", got)
	}
	if _, err := f.state.GetState(ctx, f.user.ID, "owned"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("plain state: want ErrNotFound, got %v", err)
	}
}
