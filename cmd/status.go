/*
Copyright © 2025 NAME HERE <EMAIL ADDRESS>
*/
package cmd

import (
	"fmt"
	"math"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"github.com/jackiabishop/mileminder/internal/calc"
)

// statusCmd represents the status command
var statusCmd = &cobra.Command{
	Use:   "status",
	Short: "Show allowance usage",
	RunE: func(cmd *cobra.Command, args []string) error {
		st, err := openStore()
		if err != nil {
			return err
		}
		ctx := cmd.Context()

		// Determine vehicle ID (flag or stored default).
		carFlag, _ := cmd.Flags().GetString("car")
		carID, err := defaultVehicleID(ctx, st, carFlag)
		if err != nil {
			return err
		}

		data, err := st.GetVehicle(ctx, carID)
		if err != nil {
			return err
		}

		// Canonical status math lives in internal/calc (single source of truth,
		// shared with the web API).
		s := calc.ComputeStatus(carID, data)

		termLeftStr := fmt.Sprintf("%dy %dd", s.YearsLeftTerm, s.DaysLeftTerm)

		// Progress bar
		barLen := 10
		filled := int(math.Round(s.PercentUsed / 10.0))
		if filled < 0 {
			filled = 0
		} else if filled > barLen {
			filled = barLen
		}
		bar := strings.Repeat("█", filled) + strings.Repeat("░", barLen-filled)

		// Print status
		fmt.Printf("📅 %s  | 🚗 %s\n", time.Now().Format("02 Jan 2006"), carID)
		fmt.Println(strings.Repeat("─", 50))
		fmt.Printf("Actual Odo:     %d mi\n", s.LatestReading)
		fmt.Printf("Target Today:   %.0f mi\n", s.TargetToday)
		icon := "✅"
		if s.Delta > 0 {
			icon = "⚠️"
		}
		sign := ""
		if s.Delta > 0 {
			sign = "+"
		}
		fmt.Printf("Delta:          %s%.0f mi  %s (%.0f%%)\n\n", sign, s.Delta, icon, s.PercentUsed)
		fmt.Printf("Year left:      %d d   %.0f mi\n", s.DaysLeftYear, s.MilesLeftYear)
		fmt.Printf("Term left:      %s   %.0f mi\n", termLeftStr, s.MilesLeftTerm)
		fmt.Printf("Usage:   |%s| %.0f%%\n", bar, s.PercentUsed)

		return nil
	},
}

func init() {
	rootCmd.AddCommand(statusCmd)
}
