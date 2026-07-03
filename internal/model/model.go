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
