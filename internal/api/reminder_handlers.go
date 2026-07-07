package api

import (
	"encoding/json"
	"errors"
	"net/http"

	"github.com/jackiabishop/mileminder/internal/alerts"
)

// reminderAPI serves per-vehicle reading-reminder settings. Hosted-only: the
// routes are registered solely by the hosted router.
type reminderAPI struct {
	settings alerts.ReminderSettingsStore
}

// requireVehicle confirms the vehicle exists in the caller's scoped store and
// returns its id, writing a store error and reporting false otherwise. Reminder
// settings live outside the vehicle document, so we gate on the vehicle here to
// avoid persisting settings for ids the user does not own.
func (a *reminderAPI) requireVehicle(w http.ResponseWriter, r *http.Request) (string, bool) {
	id := r.PathValue("id")
	if id == "" {
		http.Error(w, "vehicle ID required", http.StatusBadRequest)
		return "", false
	}
	if _, err := storeFrom(r.Context()).GetVehicle(r.Context(), id); err != nil {
		writeStoreError(w, err)
		return "", false
	}
	return id, true
}

func (a *reminderAPI) HandleGetReminder(w http.ResponseWriter, r *http.Request) {
	id, ok := a.requireVehicle(w, r)
	if !ok {
		return
	}
	userID := userIDFrom(r.Context())
	settings, err := a.settings.GetReminder(r.Context(), userID, id)
	if errors.Is(err, alerts.ErrNotFound) {
		s := alerts.DefaultReminderSettings(userID, id)
		settings = &s
	} else if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(settings)
}

func (a *reminderAPI) HandlePutReminder(w http.ResponseWriter, r *http.Request) {
	id, ok := a.requireVehicle(w, r)
	if !ok {
		return
	}
	userID := userIDFrom(r.Context())

	current, err := a.settings.GetReminder(r.Context(), userID, id)
	if errors.Is(err, alerts.ErrNotFound) {
		s := alerts.DefaultReminderSettings(userID, id)
		current = &s
	} else if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	var req struct {
		Enabled        *bool   `json:"enabled"`
		Frequency      *string `json:"frequency"`
		CustomInterval *int    `json:"custom_interval"`
		CustomUnit     *string `json:"custom_unit"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeValidationError(w, "invalid_json", err.Error())
		return
	}
	if req.Enabled != nil {
		current.Enabled = *req.Enabled
	}
	if req.Frequency != nil {
		current.Frequency = *req.Frequency
	}
	if req.CustomInterval != nil {
		current.CustomInterval = *req.CustomInterval
	}
	if req.CustomUnit != nil {
		current.CustomUnit = *req.CustomUnit
	}
	current.UserID = userID
	current.VehicleID = id

	if err := current.Validate(); err != nil {
		writeValidationError(w, "invalid_reminder", err.Error())
		return
	}
	if err := a.settings.PutReminder(r.Context(), *current); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(current)
}
