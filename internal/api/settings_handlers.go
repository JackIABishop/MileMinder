package api

import (
	"encoding/json"
	"net/http"
	"strings"
)

// supportedCurrencies is the curated set of ISO 4217 codes the API accepts.
// An allowlist (rather than a format check) guarantees every stored code is
// one the web client's Intl.NumberFormat can format and its picker can offer.
// Must stay in sync with SUPPORTED_CURRENCIES in web/src/lib/money.ts.
var supportedCurrencies = map[string]bool{
	"GBP": true, "USD": true, "EUR": true, "JPY": true, "CHF": true,
	"CAD": true, "AUD": true, "NZD": true, "SEK": true, "NOK": true,
	"DKK": true, "PLN": true, "ZAR": true, "INR": true,
}

// HandleGetSettings returns the user-level preferences (defaults when none
// have been saved).
func (s *Server) HandleGetSettings(w http.ResponseWriter, r *http.Request) {
	settings, err := storeFrom(r.Context()).GetSettings(r.Context())
	if err != nil {
		writeStoreError(w, err)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(settings)
}

// HandlePutSettings partially updates the preferences: omitted fields are left
// as they are (same pointer-body pattern as HandlePutPrefs).
func (s *Server) HandlePutSettings(w http.ResponseWriter, r *http.Request) {
	store := storeFrom(r.Context())
	current, err := store.GetSettings(r.Context())
	if err != nil {
		writeStoreError(w, err)
		return
	}

	var req struct {
		Currency     *string `json:"currency"`
		DistanceUnit *string `json:"distance_unit"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeValidationError(w, "invalid_json", err.Error())
		return
	}
	if req.Currency != nil {
		code := strings.ToUpper(strings.TrimSpace(*req.Currency))
		if !supportedCurrencies[code] {
			writeValidationError(w, "invalid_currency", "currency must be a supported ISO 4217 code")
			return
		}
		current.Currency = code
	}
	if req.DistanceUnit != nil {
		if *req.DistanceUnit != "mi" {
			writeValidationError(w, "invalid_distance_unit", `only "mi" is supported; km support is planned`)
			return
		}
		current.DistanceUnit = *req.DistanceUnit
	}

	if err := store.SaveSettings(r.Context(), current); err != nil {
		writeStoreError(w, err)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(current)
}
