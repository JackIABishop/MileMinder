/*
Copyright © 2025 NAME HERE <EMAIL ADDRESS>
*/
package cmd

import (
	"fmt"

	"github.com/spf13/cobra"

	"github.com/jackiabishop/mileminder/internal/calc"
)

// fleetCmd represents the fleet command
var fleetCmd = &cobra.Command{
	Use:   "fleet",
	Short: "Summary view across all vehicles",
	RunE: func(cmd *cobra.Command, args []string) error {
		st, err := openStore()
		if err != nil {
			return err
		}

		records, err := st.ListVehicles(cmd.Context())
		if err != nil {
			return err
		}
		if len(records) == 0 {
			fmt.Println("No vehicles found. Have you run `mileminder init`?")
			return nil
		}

		fmt.Printf("%-12s %-8s %-10s %-7s %s\n", "Vehicle", "Odometer", "Delta(mi)", "%Used", "TermLeft")
		for _, r := range records {
			// Canonical status math lives in internal/calc. Note delta is
			// positive when over budget (matches the web dashboard).
			s := calc.ComputeStatus(r.ID, r.Data)
			termLeft := fmt.Sprintf("%dy %dd", s.YearsLeftTerm, s.DaysLeftTerm)

			fmt.Printf("%-12s %-8d %+10.0f %7.1f%% %s\n",
				r.ID, s.LatestReading, s.Delta, s.PercentUsed, termLeft)
		}
		return nil
	},
}

func init() {
	rootCmd.AddCommand(fleetCmd)
}
