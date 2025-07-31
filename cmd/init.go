/*
Copyright © 2025 NAME HERE <EMAIL ADDRESS>
*/
package cmd

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/spf13/cobra"
	"gopkg.in/yaml.v3"

	"github.com/jackiabishop/mileminder/internal/model"
)

// initCmd represents the init command
var initCmd = &cobra.Command{
	Use:   "init --car <id>",
	Short: "Initialize a new vehicle plan",
	Args:  cobra.NoArgs,
	RunE: func(cmd *cobra.Command, args []string) error {
		carID, _ := cmd.Flags().GetString("car")
		if carID == "" {
			return fmt.Errorf("please provide a vehicle ID with --car")
		}

		reader := bufio.NewReader(os.Stdin)
		prompt := func(msg string) (string, error) {
			fmt.Print(msg)
			input, err := reader.ReadString('\n')
			return strings.TrimSpace(input), err
		}

		startStr, err := prompt("Plan start date (YYYY-MM-DD): ")
		if err != nil {
			return err
		}
		startDate, err := time.Parse("2006-01-02", startStr)
		if err != nil {
			return err
		}

		endStr, err := prompt("Plan end date (YYYY-MM-DD): ")
		if err != nil {
			return err
		}
		endDate, err := time.Parse("2006-01-02", endStr)
		if err != nil {
			return err
		}

		startMilesStr, err := prompt("Start miles: ")
		if err != nil {
			return err
		}
		startMiles, err := strconv.Atoi(startMilesStr)
		if err != nil {
			return err
		}

		annualStr, err := prompt("Annual allowance (miles): ")
		if err != nil {
			return err
		}
		annual, err := strconv.Atoi(annualStr)
		if err != nil {
			return err
		}

		data := model.VehicleData{
			Vehicle: carID,
			Plan: model.Plan{
				Start:           startDate,
				End:             endDate,
				AnnualAllowance: annual,
				StartMiles:      startMiles,
			},
			Readings: map[string]int{
				startDate.Format("2006-01-02"): startMiles,
			},
		}

		home, err := os.UserHomeDir()
		if err != nil {
			return err
		}
		dir := filepath.Join(home, ".mileminder")
		if err := os.MkdirAll(dir, 0755); err != nil {
			return err
		}

		filePath := filepath.Join(dir, carID+".yml")
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

		fmt.Printf("Created plan for %s at %s\n", carID, filePath)
		return nil
	},
}

func init() {
	rootCmd.AddCommand(initCmd)
	initCmd.Flags().StringP("car", "c", "", "Vehicle ID")
}
