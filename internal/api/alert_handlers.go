package api

import (
	"encoding/json"
	"errors"
	"net/http"

	"github.com/jackiabishop/mileminder/internal/alerts"
)

type alertPrefsAPI struct {
	prefs alerts.PrefsStore
}

func (a *alertPrefsAPI) HandleGetPrefs(w http.ResponseWriter, r *http.Request) {
	userID := userIDFrom(r.Context())
	prefs, err := a.prefs.GetPrefs(r.Context(), userID)
	if errors.Is(err, alerts.ErrNotFound) {
		p := alerts.DefaultPrefs(userID)
		prefs = &p
	} else if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(prefs)
}

func (a *alertPrefsAPI) HandlePutPrefs(w http.ResponseWriter, r *http.Request) {
	userID := userIDFrom(r.Context())
	current, err := a.prefs.GetPrefs(r.Context(), userID)
	if errors.Is(err, alerts.ErrNotFound) {
		p := alerts.DefaultPrefs(userID)
		current = &p
	} else if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	var req struct {
		Enabled   *bool    `json:"enabled"`
		Threshold *float64 `json:"threshold"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeValidationError(w, "invalid_json", err.Error())
		return
	}
	if req.Enabled != nil {
		current.Enabled = *req.Enabled
	}
	if req.Threshold != nil {
		if *req.Threshold <= 0 {
			writeValidationError(w, "invalid_threshold", "threshold must be greater than 0")
			return
		}
		current.Threshold = *req.Threshold
	}
	current.UserID = userID

	if err := a.prefs.PutPrefs(r.Context(), *current); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(current)
}
