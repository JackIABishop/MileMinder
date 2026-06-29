/*
Copyright © 2025 NAME HERE <EMAIL ADDRESS>
*/
package cmd

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"
	"gopkg.in/yaml.v3"

	"github.com/jackiabishop/mileminder/internal/calc"
	"github.com/jackiabishop/mileminder/internal/model"
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
		dir := filepath.Join(home, ".mileminder")
		entries, err := os.ReadDir(dir)
		if err != nil {
			if os.IsNotExist(err) {
				fmt.Println("No vehicles found. Have you run `mileminder init`?")
				return nil
			}
			return err
		}

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

			// Canonical status math lives in internal/calc. Note delta is
			// positive when over budget (matches the web dashboard).
			s := calc.ComputeStatus(id, &data)
			termLeft := fmt.Sprintf("%dy %dd", s.YearsLeftTerm, s.DaysLeftTerm)

			fmt.Printf("%-12s %-8d %+10.0f %7.1f%% %s\n",
				id, s.LatestReading, s.Delta, s.PercentUsed, termLeft)
		}
		return nil
	},
}

func init() {
	rootCmd.AddCommand(fleetCmd)
}
