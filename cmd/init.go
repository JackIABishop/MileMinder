/*
Copyright © 2025 NAME HERE <EMAIL ADDRESS>
*/
package cmd

import (
	"bufio"
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/spf13/cobra"

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
		noPlan, _ := cmd.Flags().GetBool("no-plan")
		prompt := func(msg string) (string, error) {
			fmt.Print(msg)
			input, err := reader.ReadString('\n')
			return strings.TrimSpace(input), err
		}

		data := model.VehicleData{
			Vehicle: carID,
			Readings: map[string]int{
				time.Now().Format("2006-01-02"): 0,
			},
		}
		if noPlan {
			startMilesStr, err := prompt("Current odometer: ")
			if err != nil {
				return err
			}
			startMiles, err := strconv.Atoi(startMilesStr)
			if err != nil {
				return err
			}
			data.Readings = map[string]int{
				time.Now().Format("2006-01-02"): startMiles,
			}
		} else {
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

			excessRate, _ := cmd.Flags().GetInt("excess-rate")
			data.Plan = &model.Plan{
				Start:           startDate,
				End:             endDate,
				AnnualAllowance: annual,
				StartMiles:      startMiles,
				ExcessRate:      excessRate,
			}
			data.Readings = map[string]int{
				startDate.Format("2006-01-02"): startMiles,
			}
		}

		st, err := openStore()
		if err != nil {
			return err
		}
		if err := st.SaveVehicle(cmd.Context(), carID, &data); err != nil {
			return err
		}

		if noPlan {
			fmt.Printf("Created tracker for %s\n", carID)
		} else {
			fmt.Printf("Created plan for %s\n", carID)
		}
		return nil
	},
}

func init() {
	rootCmd.AddCommand(initCmd)
	initCmd.Flags().StringP("car", "c", "", "Vehicle ID")
	initCmd.Flags().Int("excess-rate", 0, "Excess mileage penalty in pence per mile over allowance (optional)")
	initCmd.Flags().Bool("no-plan", false, "Create a plan-less mileage tracker")
}
