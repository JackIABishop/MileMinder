/*
Copyright © 2025 NAME HERE <EMAIL ADDRESS>
*/
package cmd

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"time"

	"github.com/spf13/cobra"
	"gopkg.in/yaml.v3"

	"github.com/jackbishop/mileage-cli/internal/model"
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
		// Load existing data
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
		// Validate against max existing reading
		maxMiles := 0
		for _, m := range data.Readings {
			if m > maxMiles {
				maxMiles = m
			}
		}
		if miles < maxMiles && !force {
			return fmt.Errorf("new reading %d is less than existing max %d; use --force to override", miles, maxMiles)
		}
		// Upsert reading
		data.Readings[dateStr] = miles
		// Write back
		f, err := os.Create(filePath)
		if err != nil {
			return err
		}
		defer f.Close()
		enc := yaml.NewEncoder(f)
		defer enc.Close()
		if err := enc.Encode(&data); err != nil {
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
