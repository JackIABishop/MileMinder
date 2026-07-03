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
// sends edge-triggered alerts.
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
	if !prefs.Enabled {
		return
	}

	records, err := s.Tenants.ForUser(u.ID).ListVehicles(ctx)
	if err != nil {
		s.logf("alerts: list vehicles for user %s: %v", u.ID, err)
		return
	}
	keep := make([]string, 0, len(records))
	for _, rec := range records {
		keep = append(keep, rec.ID)
		s.runVehicle(ctx, u, prefs, rec)
	}
	if pruner, ok := s.State.(PruningStateStore); ok {
		if err := pruner.PruneUserStates(ctx, u.ID, keep); err != nil {
			s.logf("alerts: prune states for user %s: %v", u.ID, err)
		}
	}
}

func (s *Scheduler) runVehicle(ctx context.Context, u *auth.User, prefs *Prefs, rec storage.Record) {
	if rec.Data == nil || !rec.Data.HasPlan() {
		return
	}
	now := s.now()
	status := calc.ComputeStatusAt(rec.ID, rec.Data, now)
	breach := calc.EvaluateBreach(status, prefs.Threshold)
	isBreached := breach.Breached()

	prev, err := s.State.GetState(ctx, u.ID, rec.ID)
	if errors.Is(err, ErrNotFound) {
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
