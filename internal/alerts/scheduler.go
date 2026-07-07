package alerts

import (
	"context"
	"errors"
	"log"
	"time"

	"github.com/jackiabishop/mileminder/internal/auth"
	"github.com/jackiabishop/mileminder/internal/calc"
	"github.com/jackiabishop/mileminder/internal/notify"
	"github.com/jackiabishop/mileminder/internal/storage"
)

// Scheduler periodically sweeps hosted users, computes vehicle statuses and
// sends edge-triggered breach alerts plus time-based reading reminders.
type Scheduler struct {
	Users    auth.UserStore
	Tenants  storage.Tenants
	State    StateStore
	Prefs    PrefsStore
	Channel  notify.Channel
	Now      func() time.Time
	Interval time.Duration
	BaseURL  string
	Logger   *log.Logger

	// Reminders and ReminderState drive the reading-reminder pass. Both nil (the
	// default) disables reminders entirely, leaving breach alerting unchanged.
	Reminders     ReminderSettingsStore
	ReminderState ReminderStateStore
}

// Run executes RunOnce immediately, then on Interval until ctx is cancelled.
func (s *Scheduler) Run(ctx context.Context) {
	interval := s.Interval
	if interval <= 0 {
		interval = time.Hour
	}
	s.RunOnce(ctx)
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			s.RunOnce(ctx)
		}
	}
}

// RunOnce performs one complete alert sweep. Per-user and per-vehicle errors
// are logged and skipped so a single bad record does not abort the sweep.
func (s *Scheduler) RunOnce(ctx context.Context) {
	if s.Users == nil || s.Tenants == nil || s.State == nil || s.Prefs == nil || s.Channel == nil {
		s.logf("alerts: scheduler missing dependency")
		return
	}
	users, err := s.Users.ListUsers(ctx)
	if err != nil {
		s.logf("alerts: list users: %v", err)
		return
	}
	for _, u := range users {
		s.runUser(ctx, u)
	}
}

func (s *Scheduler) runUser(ctx context.Context, u *auth.User) {
	prefs, err := s.Prefs.GetPrefs(ctx, u.ID)
	if errors.Is(err, ErrNotFound) {
		p := DefaultPrefs(u.ID)
		prefs = &p
	} else if err != nil {
		s.logf("alerts: load prefs for user %s: %v", u.ID, err)
		return
	}

	records, err := s.Tenants.ForUser(u.ID).ListVehicles(ctx)
	if err != nil {
		s.logf("alerts: list vehicles for user %s: %v", u.ID, err)
		return
	}
	now := s.now()
	remindersOn := s.Reminders != nil && s.ReminderState != nil
	keep := make([]string, 0, len(records))
	for _, rec := range records {
		keep = append(keep, rec.ID)
		if rec.Data == nil {
			continue
		}
		// Status is the single source of truth for both passes; compute it once.
		status := calc.ComputeStatusAt(rec.ID, rec.Data, now)
		if prefs.Enabled {
			s.runVehicle(ctx, u, prefs, rec, status, now)
		}
		if remindersOn {
			s.runReminder(ctx, u, rec, status, now)
		}
	}
	if pruner, ok := s.State.(PruningStateStore); ok {
		if err := pruner.PruneUserStates(ctx, u.ID, keep); err != nil {
			s.logf("alerts: prune states for user %s: %v", u.ID, err)
		}
	}
	if remindersOn {
		// Prune only derived reminder state, never user-configured settings.
		// keep is built from ListVehicles, which deliberately skips unreadable or
		// unparseable vehicle files (see yamlstore.ListVehicles); pruning settings
		// against it would let a transient read/parse glitch silently and
		// permanently delete a user's reminder config for a vehicle that still
		// exists. Losing derived state (last_reminded_at) self-heals — at worst one
		// duplicate reminder — so it is safe to prune here. Settings cleanup belongs
		// on an explicit vehicle-delete path (none exists in hosted mode yet);
		// PruneUserReminders stays on the store for that future use.
		if err := s.ReminderState.PruneUserReminderStates(ctx, u.ID, keep); err != nil {
			s.logf("alerts: prune reminder states for user %s: %v", u.ID, err)
		}
	}
}

func (s *Scheduler) runVehicle(ctx context.Context, u *auth.User, prefs *Prefs, rec storage.Record, status calc.Status, now time.Time) {
	if !rec.Data.HasPlan() {
		return
	}
	breach := calc.EvaluateBreach(status, prefs.Threshold)
	isBreached := breach.Breached()

	prev, err := s.State.GetState(ctx, u.ID, rec.ID)
	if errors.Is(err, ErrNotFound) {
		// First observation is a baseline for this user/vehicle pair, not a
		// crossing. This also applies to vehicles added later already breached.
		if err := s.State.PutState(ctx, VehicleAlertState{UserID: u.ID, VehicleID: rec.ID, Breached: isBreached}); err != nil {
			s.logf("alerts: seed state for user %s vehicle %s: %v", u.ID, rec.ID, err)
		}
		return
	}
	if err != nil {
		s.logf("alerts: load state for user %s vehicle %s: %v", u.ID, rec.ID, err)
		return
	}

	if !prev.Breached && isBreached {
		msg, err := RenderBreachMessage(status, breach, s.BaseURL)
		if err != nil {
			s.logf("alerts: render message for user %s vehicle %s: %v", u.ID, rec.ID, err)
			return
		}
		if err := s.Channel.Send(ctx, notify.Recipient{Email: u.Email}, msg); err != nil {
			s.logf("alerts: send message for user %s vehicle %s: %v", u.ID, rec.ID, err)
			return
		}
		prev.Breached = true
		prev.LastAlertedAt = now
		if err := s.State.PutState(ctx, *prev); err != nil {
			s.logf("alerts: persist sent state for user %s vehicle %s: %v", u.ID, rec.ID, err)
		}
		return
	}

	if prev.Breached != isBreached {
		prev.Breached = isBreached
		if err := s.State.PutState(ctx, *prev); err != nil {
			s.logf("alerts: persist clear state for user %s vehicle %s: %v", u.ID, rec.ID, err)
		}
	}
}

// runReminder sends a time-based reading reminder when the vehicle has not been
// logged within the configured interval. Unlike breach alerts it is not
// edge-triggered: as long as the reading stays stale it re-fires once per
// interval, anchored on the later of the last reading and the last reminder.
func (s *Scheduler) runReminder(ctx context.Context, u *auth.User, rec storage.Record, status calc.Status, now time.Time) {
	settings, err := s.Reminders.GetReminder(ctx, u.ID, rec.ID)
	if errors.Is(err, ErrNotFound) {
		return // unconfigured vehicles default to reminders off
	}
	if err != nil {
		s.logf("alerts: load reminder for user %s vehicle %s: %v", u.ID, rec.ID, err)
		return
	}
	if !settings.Enabled {
		return
	}
	interval := settings.IntervalDays()
	if interval <= 0 {
		s.logf("alerts: invalid reminder interval for user %s vehicle %s: frequency=%q", u.ID, rec.ID, settings.Frequency)
		return
	}

	// Baseline is the last logged reading; a policy vehicle with no readings
	// falls back to its plan start, and a plain vehicle with no readings has
	// nothing to anchor on and is skipped.
	var readingDate time.Time
	if status.LatestDate != "" {
		t, perr := time.Parse("2006-01-02", status.LatestDate)
		if perr != nil {
			s.logf("alerts: parse latest date for user %s vehicle %s: %v", u.ID, rec.ID, perr)
			return
		}
		readingDate = t
	} else if rec.Data.HasPlan() {
		readingDate = rec.Data.Plan.Start
	} else {
		return
	}

	// Trigger on the later of the reading baseline and the last reminder we sent,
	// so a fresh reading or a just-sent reminder both reset the countdown.
	triggerFrom := readingDate
	state, err := s.ReminderState.GetReminderState(ctx, u.ID, rec.ID)
	if err != nil && !errors.Is(err, ErrNotFound) {
		s.logf("alerts: load reminder state for user %s vehicle %s: %v", u.ID, rec.ID, err)
		return
	}
	if state != nil && state.LastRemindedAt.After(triggerFrom) {
		triggerFrom = state.LastRemindedAt
	}
	if now.Sub(triggerFrom) < time.Duration(interval)*24*time.Hour {
		return
	}

	daysSince := int(now.Sub(readingDate).Hours() / 24)
	msg, err := RenderReminderMessage(status, daysSince, s.BaseURL)
	if err != nil {
		s.logf("alerts: render reminder for user %s vehicle %s: %v", u.ID, rec.ID, err)
		return
	}
	if err := s.Channel.Send(ctx, notify.Recipient{Email: u.Email}, msg); err != nil {
		s.logf("alerts: send reminder for user %s vehicle %s: %v", u.ID, rec.ID, err)
		return
	}
	if err := s.ReminderState.PutReminderState(ctx, VehicleReminderState{UserID: u.ID, VehicleID: rec.ID, LastRemindedAt: now}); err != nil {
		s.logf("alerts: persist reminder state for user %s vehicle %s: %v", u.ID, rec.ID, err)
	}
}

func (s *Scheduler) now() time.Time {
	if s.Now != nil {
		return s.Now()
	}
	return time.Now()
}

func (s *Scheduler) logf(format string, args ...any) {
	logger := s.Logger
	if logger == nil {
		logger = log.Default()
	}
	logger.Printf(format, args...)
}
