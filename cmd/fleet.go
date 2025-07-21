/*
Copyright © 2025 NAME HERE <EMAIL ADDRESS>
*/
package cmd

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/spf13/cobra"
	"gopkg.in/yaml.v3"

	"github.com/jackbishop/mileage-cli/internal/model"
)

// fleetCmd represents the fleet command
var fleetCmd = &cobra.Command{
	Use:   "fleet",
	Short: "Summary view across all vehicles",
	RunE: func(cmd *cobra.Command, args []string) error {
		home, err := os.UserHomeDir()
		if err != nil {
			return fmt.Errorf("unable to find home directory: %w", err)
		}
		dir := filepath.Join(home, ".mileage-cli")
		entries, err := os.ReadDir(dir)
		if err != nil {
			if os.IsNotExist(err) {
				fmt.Println("No vehicles found. Have you run `mileage init`?")
				return nil
			}
			return err
		}

		today := time.Now()
		fmt.Printf("%-12s %-8s %-10s %-7s %s\n", "Vehicle", "Odometer", "Delta(mi)", "%Used", "TermLeft")
		for _, e := range entries {
			if e.IsDir() {
				continue
			}
			name := e.Name()
			if filepath.Ext(name) != ".yml" {
				continue
			}
			id := strings.TrimSuffix(name, ".yml")

			raw, err := os.ReadFile(filepath.Join(dir, name))
			if err != nil {
				return err
			}
			var data model.VehicleData
			if err := yaml.Unmarshal(raw, &data); err != nil {
				return err
			}

			// Determine latest reading
			latest := 0
			for _, m := range data.Readings {
				if m > latest {
					latest = m
				}
			}

			// Compute metrics
			daysElapsed := int(today.Sub(data.Plan.Start).Hours() / 24)
			if daysElapsed < 0 {
				daysElapsed = 0
			}
			allowedToday := float64(data.Plan.AnnualAllowance) * float64(daysElapsed) / 365.0
			milesUsed := float64(latest - data.Plan.StartMiles)
			delta := allowedToday - milesUsed
			var percent float64
			if allowedToday > 0 {
				percent = milesUsed / allowedToday * 100.0
			}

			termDays := int(data.Plan.End.Sub(today).Hours() / 24)
			if termDays < 0 {
				termDays = 0
			}
			yearsLeft := termDays / 365
			daysLeft := termDays % 365
			termLeft := fmt.Sprintf("%dy %dd", yearsLeft, daysLeft)

			fmt.Printf("%-12s %-8d %+10.0f %7.1f%% %s\n",
				id, latest, delta, percent, termLeft)
		}
		return nil
	},
}

func init() {
	rootCmd.AddCommand(fleetCmd)
}
