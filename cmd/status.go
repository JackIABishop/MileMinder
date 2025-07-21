/*
Copyright © 2025 NAME HERE <EMAIL ADDRESS>
*/
package cmd

import (
	"fmt"
	"math"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/spf13/cobra"
	"gopkg.in/yaml.v3"

	"github.com/jackbishop/mileage-cli/internal/model"
)

// statusCmd represents the status command
var statusCmd = &cobra.Command{
	Use:   "status",
	Short: "Show allowance usage",
	RunE: func(cmd *cobra.Command, args []string) error {
		// Determine vehicle ID (flag or default)
		carID, _ := cmd.Flags().GetString("car")
		if carID == "" {
			home, err := os.UserHomeDir()
			if err != nil {
				return err
			}
			data, err := os.ReadFile(filepath.Join(home, ".mileage-cli", "current"))
			if err != nil {
				return fmt.Errorf("no vehicle specified and no default set; use --car or switch")
			}
			carID = strings.TrimSpace(string(data))
		}

		// Load vehicle data
		home, err := os.UserHomeDir()
		if err != nil {
			return err
		}
		filePath := filepath.Join(home, ".mileage-cli", carID+".yml")
		raw, err := os.ReadFile(filePath)
		if err != nil {
			return err
		}
		var data model.VehicleData
		if err := yaml.Unmarshal(raw, &data); err != nil {
			return err
		}

		// Find latest reading
		var dates []string
		for d := range data.Readings {
			dates = append(dates, d)
		}
		sort.Strings(dates)
		latestDate := dates[len(dates)-1]
		latestMiles := data.Readings[latestDate]

		today := time.Now()
		// Compute target vs actual
		daysElapsed := today.Sub(data.Plan.Start).Hours() / 24.0
		if daysElapsed < 0 {
			daysElapsed = 0
		}
		targetToday := float64(data.Plan.StartMiles) + float64(data.Plan.AnnualAllowance)*daysElapsed/365.0
		milesUsed := float64(latestMiles - data.Plan.StartMiles)
		delta := milesUsed - targetToday
		var pctUsed float64
		if targetToday > 0 {
			pctUsed = milesUsed / targetToday * 100.0
		}

		// Year left (plan-year segment)
		// Determine the current plan-year window (from last anniversary to next)
		yearsSince := today.Year() - data.Plan.Start.Year()
		segmentStart := data.Plan.Start.AddDate(yearsSince, 0, 0)
		if segmentStart.After(today) {
			segmentStart = segmentStart.AddDate(-1, 0, 0)
		}
		segmentEnd := segmentStart.AddDate(1, 0, 0)
		if segmentEnd.After(data.Plan.End) {
			segmentEnd = data.Plan.End
		}
		// Compute days and miles left in this plan year
		daysLeftYear := segmentEnd.Sub(today).Hours() / 24.0
		if daysLeftYear < 0 {
			daysLeftYear = 0
		}
		milesLeftYear := float64(data.Plan.AnnualAllowance) * daysLeftYear / 365.0

		// Term left
		termDays := data.Plan.End.Sub(today).Hours() / 24.0
		if termDays < 0 {
			termDays = 0
		}
		yearsLeft := int(termDays / 365.0)
		daysLeft := int(math.Mod(termDays, 365.0))
		termLeftStr := fmt.Sprintf("%dy %dd", yearsLeft, daysLeft)
		milesLeftTerm := float64(data.Plan.AnnualAllowance) * termDays / 365.0

		// Progress bar
		barLen := 10
		filled := int(math.Round(pctUsed / 10.0))
		if filled < 0 {
			filled = 0
		} else if filled > barLen {
			filled = barLen
		}
		bar := strings.Repeat("█", filled) + strings.Repeat("░", barLen-filled)

		// Print status
		fmt.Printf("📅 %s  | 🚗 %s\n", today.Format("02 Jan 2006"), carID)
		fmt.Println(strings.Repeat("─", 50))
		fmt.Printf("Actual Odo:     %d mi\n", latestMiles)
		fmt.Printf("Target Today:   %.0f mi\n", targetToday)
		icon := "✅"
		if delta > 0 {
			icon = "⚠️"
		}
		sign := ""
		if delta > 0 {
			sign = "+"
		}
		fmt.Printf("Delta:          %s%.0f mi  %s (%.0f%%)\n\n", sign, delta, icon, pctUsed)
		fmt.Printf("Year left:      %d d   %.0f mi\n", int(math.Ceil(daysLeftYear)), milesLeftYear)
		fmt.Printf("Term left:      %s   %.0f mi\n", termLeftStr, milesLeftTerm)
		fmt.Printf("Usage:   |%s| %.0f%%\n", bar, pctUsed)

		return nil
	},
}

func init() {
	rootCmd.AddCommand(statusCmd)
}
