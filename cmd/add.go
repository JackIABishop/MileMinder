/*
Copyright © 2025 NAME HERE <EMAIL ADDRESS>
*/
package cmd

import (
	"fmt"
	"strconv"
	"time"

	"github.com/spf13/cobra"

	"github.com/jackiabishop/mileminder/internal/readings"
)

var addCmd = &cobra.Command{
	Use:   "add <odometer>",
	Short: "Record today's odometer reading",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		// Get vehicle ID
		carID, _ := cmd.Flags().GetString("car")
		if carID == "" {
			return fmt.Errorf("please provide a vehicle ID with --car")
		}
		// Determine date
		dateStr, _ := cmd.Flags().GetString("date")
		if dateStr == "" {
			dateStr = time.Now().Format("2006-01-02")
		} else {
			if _, err := time.Parse("2006-01-02", dateStr); err != nil {
				return fmt.Errorf("invalid date: %v", err)
			}
		}
		// Parse flags
		force, _ := cmd.Flags().GetBool("force")
		miles, err := strconv.Atoi(args[0])
		if err != nil {
			return fmt.Errorf("invalid odometer value: %v", err)
		}

		st, err := openStore()
		if err != nil {
			return err
		}
		ctx := cmd.Context()

		// Load existing data to validate against the current max reading.
		data, err := st.GetVehicle(ctx, carID)
		if err != nil {
			return err
		}
		if max, below := readings.BelowMax(data.Readings, miles); below && !force {
			return fmt.Errorf("new reading %d is less than existing max %d; use --force to override", miles, max)
		}

		if err := st.PutReading(ctx, carID, dateStr, miles); err != nil {
			return err
		}
		fmt.Printf("Recorded odometer reading %d for %s on %s\n", miles, carID, dateStr)
		return nil
	},
}

func init() {
	rootCmd.AddCommand(addCmd)
	addCmd.Flags().StringP("car", "c", "", "Vehicle ID")
	addCmd.Flags().String("date", "", "Date for reading (YYYY-MM-DD), default today")
	addCmd.Flags().Bool("force", false, "Allow lower-than-previous readings")
}
