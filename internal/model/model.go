package model

import (
	"time"
)

type Plan struct {
	Start           time.Time `yaml:"start" json:"start"`
	End             time.Time `yaml:"end" json:"end"`
	AnnualAllowance int       `yaml:"annual_allowance" json:"annual_allowance"`
	StartMiles      int       `yaml:"start_miles" json:"start_miles"`
	ExcessRate      int       `yaml:"excess_rate,omitempty" json:"excess_rate,omitempty"` // pence per excess mile
}

type VehicleData struct {
	Vehicle  string         `yaml:"vehicle" json:"vehicle"`
	Plan     *Plan          `yaml:"plan,omitempty" json:"plan,omitempty"`
	Readings map[string]int `yaml:"readings" json:"readings"` // date string → miles
}

func (v *VehicleData) HasPlan() bool {
	return v != nil && v.Plan != nil
}

// Settings is the user-level preferences document. Money fields across the app
// (e.g. Plan.ExcessRate) are stored in the minor unit of Currency; DistanceUnit
// records the unit distances are stored and displayed in so the document stays
// honest if km support lands, but only "mi" is accepted today.
type Settings struct {
	Currency     string `yaml:"currency" json:"currency"`           // ISO 4217; default GBP
	DistanceUnit string `yaml:"distance_unit" json:"distance_unit"` // "mi" today; recorded so the model is km-ready
}

// DefaultSettings returns the settings assumed when none have been saved —
// they match the app's historical implicit behaviour (GBP pence, miles).
func DefaultSettings() Settings {
	return Settings{Currency: "GBP", DistanceUnit: "mi"}
}
