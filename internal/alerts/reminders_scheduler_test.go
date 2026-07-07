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

type reminderFixture struct {
	users     *auth.MemoryUserStore
	tenants   *storage.MemoryTenants
	state     *MemoryStateStore
	prefs     *MemoryPrefsStore
	rSettings *MemoryReminderSettingsStore
	rState    *MemoryReminderStateStore
	fake      *notify.Fake
	user      *auth.User
	sched     *Scheduler
	now       time.Time
}

func newReminderFixture(t *testing.T, now time.Time) *reminderFixture {
	t.Helper()
	ctx := context.Background()
	users := auth.NewMemoryUserStore()
	user, err := users.CreateUser(ctx, "alice@example.com", "hash")
	if err != nil {
		t.Fatalf("CreateUser: %v", err)
	}
	f := &reminderFixture{
		users:     users,
		tenants:   storage.NewMemoryTenants(),
		state:     NewMemoryStateStore(),
		prefs:     NewMemoryPrefsStore(),
		rSettings: NewMemoryReminderSettingsStore(),
		rState:    NewMemoryReminderStateStore(),
		fake:      notify.NewFake(),
		user:      user,
		now:       now,
	}
	f.sched = &Scheduler{
		Users:         f.users,
		Tenants:       f.tenants,
		State:         f.state,
		Prefs:         f.prefs,
		Channel:       f.fake,
		Now:           func() time.Time { return f.now },
		BaseURL:       "https://app.example.com",
		Reminders:     f.rSettings,
		ReminderState: f.rState,
	}
	return f
}

func (f *reminderFixture) save(t *testing.T, id string, v *model.VehicleData) {
	t.Helper()
	if err := f.tenants.ForUser(f.user.ID).SaveVehicle(context.Background(), id, v); err != nil {
		t.Fatalf("SaveVehicle %s: %v", id, err)
	}
}

func (f *reminderFixture) enableReminder(t *testing.T, id string, r ReminderSettings) {
	t.Helper()
	r.UserID = f.user.ID
	r.VehicleID = id
	r.Enabled = true
	if err := f.rSettings.PutReminder(context.Background(), r); err != nil {
		t.Fatalf("PutReminder: %v", err)
	}
}

func plainVehicle(readings map[string]int) *model.VehicleData {
	return &model.VehicleData{Vehicle: "Owned", Readings: readings}
}

// TestReminderNoSettingsNoSend: a vehicle without reminder settings is silent.
func TestReminderNoSettingsNoSend(t *testing.T) {
	ctx := context.Background()
	f := newReminderFixture(t, alertDate("2025-04-20"))
	f.save(t, "golf", policyVehicle(5000))

	f.sched.RunOnce(ctx)

	if got := len(f.fake.Deliveries()); got != 0 {
		t.Fatalf("deliveries = %d, want 0", got)
	}
}

// TestReminderStaleReadingSends: enabled + a reading older than the interval
// sends one reminder and records the send timestamp.
func TestReminderStaleReadingSends(t *testing.T) {
	ctx := context.Background()
	f := newReminderFixture(t, alertDate("2025-04-20"))
	f.save(t, "golf", policyVehicle(5000)) // last reading 2025-04-11
	f.enableReminder(t, "golf", ReminderSettings{Frequency: FrequencyWeekly})

	f.sched.RunOnce(ctx)

	deliveries := f.fake.Deliveries()
	if len(deliveries) != 1 {
		t.Fatalf("deliveries = %d, want 1", len(deliveries))
	}
	if deliveries[0].To.Email != "alice@example.com" {
		t.Fatalf("recipient = %q", deliveries[0].To.Email)
	}
	if sub := deliveries[0].Message.Subject; !strings.Contains(sub, "log a reading") || !strings.Contains(sub, "Golf") {
		t.Fatalf("subject = %q", sub)
	}
	if body := deliveries[0].Message.Body; !strings.Contains(body, "9 days") {
		t.Fatalf("body missing day count:\n%s", body)
	}
	st, err := f.rState.GetReminderState(ctx, f.user.ID, "golf")
	if err != nil {
		t.Fatalf("GetReminderState: %v", err)
	}
	if !st.LastRemindedAt.Equal(f.now) {
		t.Fatalf("LastRemindedAt = %v, want %v", st.LastRemindedAt, f.now)
	}
}

// TestReminderFreshReadingNoSend: a reading newer than the interval is silent.
func TestReminderFreshReadingNoSend(t *testing.T) {
	ctx := context.Background()
	f := newReminderFixture(t, alertDate("2025-04-15"))
	f.save(t, "golf", policyVehicle(5000)) // last reading 2025-04-11 (4 days)
	f.enableReminder(t, "golf", ReminderSettings{Frequency: FrequencyWeekly})

	f.sched.RunOnce(ctx)

	if got := len(f.fake.Deliveries()); got != 0 {
		t.Fatalf("deliveries = %d, want 0", got)
	}
}

// TestReminderRepeatNag: still stale after another interval fires again.
func TestReminderRepeatNag(t *testing.T) {
	ctx := context.Background()
	f := newReminderFixture(t, alertDate("2025-04-20"))
	f.save(t, "golf", policyVehicle(5000))
	f.enableReminder(t, "golf", ReminderSettings{Frequency: FrequencyDaily})

	f.sched.RunOnce(ctx) // sends, records 2025-04-20
	// Same day, less than a day since the last reminder: silent.
	f.now = alertDate("2025-04-20").Add(12 * time.Hour)
	f.sched.RunOnce(ctx)
	if got := len(f.fake.Deliveries()); got != 1 {
		t.Fatalf("deliveries after 12h = %d, want 1", got)
	}
	// A full day later: fires again.
	f.now = alertDate("2025-04-22")
	f.sched.RunOnce(ctx)
	if got := len(f.fake.Deliveries()); got != 2 {
		t.Fatalf("deliveries after another day = %d, want 2", got)
	}
}

// TestReminderNewReadingResetsAnchor: logging a reading stops the nagging.
func TestReminderNewReadingResetsAnchor(t *testing.T) {
	ctx := context.Background()
	f := newReminderFixture(t, alertDate("2025-04-20"))
	f.save(t, "golf", policyVehicle(5000)) // last reading 2025-04-11
	f.enableReminder(t, "golf", ReminderSettings{Frequency: FrequencyWeekly})

	f.sched.RunOnce(ctx) // sends (9 days stale)
	if got := len(f.fake.Deliveries()); got != 1 {
		t.Fatalf("first run deliveries = %d, want 1", got)
	}
	// User logs a fresh reading on the day of the reminder.
	v := policyVehicle(6000)
	v.Readings["2025-04-20"] = 6000
	f.save(t, "golf", v)
	// Five days later, the reading is fresh (< 7 days): silent.
	f.now = alertDate("2025-04-25")
	f.sched.RunOnce(ctx)
	if got := len(f.fake.Deliveries()); got != 1 {
		t.Fatalf("deliveries after fresh reading = %d, want 1", got)
	}
}

// TestReminderSendsWhenAlertsDisabled guards the runUser restructure: reminders
// must fire even when breach alerts are turned off.
func TestReminderSendsWhenAlertsDisabled(t *testing.T) {
	ctx := context.Background()
	f := newReminderFixture(t, alertDate("2025-04-20"))
	f.save(t, "golf", policyVehicle(5000))
	if err := f.prefs.PutPrefs(ctx, Prefs{UserID: f.user.ID, Enabled: false, Threshold: 100}); err != nil {
		t.Fatalf("PutPrefs: %v", err)
	}
	f.enableReminder(t, "golf", ReminderSettings{Frequency: FrequencyWeekly})

	f.sched.RunOnce(ctx)

	if got := len(f.fake.Deliveries()); got != 1 {
		t.Fatalf("deliveries = %d, want 1 (reminder despite alerts off)", got)
	}
}

// TestReminderPlainVehicle: tracking-only vehicles get reminders too.
func TestReminderPlainVehicle(t *testing.T) {
	ctx := context.Background()
	f := newReminderFixture(t, alertDate("2025-04-20"))
	f.save(t, "owned", plainVehicle(map[string]int{"2025-01-01": 10000, "2025-04-11": 20000}))
	f.enableReminder(t, "owned", ReminderSettings{Frequency: FrequencyWeekly})

	f.sched.RunOnce(ctx)

	if got := len(f.fake.Deliveries()); got != 1 {
		t.Fatalf("deliveries = %d, want 1", got)
	}
}

// TestReminderPolicyVehicleNoReadings anchors on plan.Start when there are no
// readings yet.
func TestReminderPolicyVehicleNoReadings(t *testing.T) {
	ctx := context.Background()
	f := newReminderFixture(t, alertDate("2025-02-01"))
	v := &model.VehicleData{
		Vehicle: "Golf",
		Plan: &model.Plan{
			Start:           alertDate("2025-01-01"),
			End:             alertDate("2026-01-01"),
			AnnualAllowance: 10000,
			StartMiles:      0,
		},
		Readings: map[string]int{},
	}
	f.save(t, "golf", v)
	f.enableReminder(t, "golf", ReminderSettings{Frequency: FrequencyWeekly})

	f.sched.RunOnce(ctx)

	deliveries := f.fake.Deliveries()
	if len(deliveries) != 1 {
		t.Fatalf("deliveries = %d, want 1", len(deliveries))
	}
	if body := deliveries[0].Message.Body; !strings.Contains(body, "haven't logged any readings") {
		t.Fatalf("body missing no-reading copy:\n%s", body)
	}
}

// TestReminderPlainVehicleNoReadingsSkipped: nothing to anchor on → silent.
func TestReminderPlainVehicleNoReadingsSkipped(t *testing.T) {
	ctx := context.Background()
	f := newReminderFixture(t, alertDate("2025-04-20"))
	f.save(t, "owned", plainVehicle(map[string]int{}))
	f.enableReminder(t, "owned", ReminderSettings{Frequency: FrequencyWeekly})

	f.sched.RunOnce(ctx)

	if got := len(f.fake.Deliveries()); got != 0 {
		t.Fatalf("deliveries = %d, want 0", got)
	}
}

// TestReminderSweepPrunesStateNotSettings: the sweep prunes derived reminder
// state for vehicles missing from ListVehicles, but must never delete
// user-configured settings — ListVehicles silently skips unreadable/unparseable
// files, so pruning settings against it would risk destroying live config.
func TestReminderSweepPrunesStateNotSettings(t *testing.T) {
	ctx := context.Background()
	f := newReminderFixture(t, alertDate("2025-04-20"))
	f.save(t, "golf", policyVehicle(5000))
	f.enableReminder(t, "golf", ReminderSettings{Frequency: FrequencyWeekly})

	f.sched.RunOnce(ctx) // sends + writes state
	if err := f.tenants.ForUser(f.user.ID).DeleteVehicle(ctx, "golf"); err != nil {
		t.Fatalf("DeleteVehicle: %v", err)
	}
	f.sched.RunOnce(ctx)

	// Derived state is pruned (self-healing).
	if _, err := f.rState.GetReminderState(ctx, f.user.ID, "golf"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("state not pruned: %v", err)
	}
	// User settings are preserved — the sweep must not delete config.
	if _, err := f.rSettings.GetReminder(ctx, f.user.ID, "golf"); err != nil {
		t.Fatalf("settings should survive the sweep, got: %v", err)
	}
}

// TestReminderNilStoresNoSend: with reminder stores unset (the pre-#52 default),
// no reminders are attempted even for a stale reading.
func TestReminderNilStoresNoSend(t *testing.T) {
	ctx := context.Background()
	f := newReminderFixture(t, alertDate("2025-04-20"))
	f.sched.Reminders = nil
	f.sched.ReminderState = nil
	f.save(t, "golf", policyVehicle(5000))
	f.enableReminder(t, "golf", ReminderSettings{Frequency: FrequencyDaily})

	f.sched.RunOnce(ctx)

	if got := len(f.fake.Deliveries()); got != 0 {
		t.Fatalf("deliveries = %d, want 0 (reminders disabled)", got)
	}
}
