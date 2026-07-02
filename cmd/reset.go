/*
Copyright © 2025 NAME HERE <EMAIL ADDRESS>
*/
package cmd

import (
	"bufio"
	"errors"
	"fmt"
	"os"
	"strings"

	"github.com/spf13/cobra"

	"github.com/jackiabishop/mileminder/internal/storage"
)

var resetCmd = &cobra.Command{
	Use:   "reset --car <id>",
	Short: "Delete all data for a vehicle",
	Args:  cobra.NoArgs,
	RunE: func(cmd *cobra.Command, args []string) error {
		carID, _ := cmd.Flags().GetString("car")
		if carID == "" {
			return fmt.Errorf("please provide a vehicle ID with --car")
		}

		st, err := openStore()
		if err != nil {
			return err
		}
		ctx := cmd.Context()

		// Confirm existence before prompting, preserving the old friendly message.
		if _, err := st.GetVehicle(ctx, carID); err != nil {
			if errors.Is(err, storage.ErrNotFound) {
				fmt.Printf("Vehicle %q not found; have you run `mileminder init`?\n", carID)
				return nil
			}
			return err
		}

		reader := bufio.NewReader(os.Stdin)
		fmt.Printf("Are you sure you want to delete data for %s? (y/N): ", carID)
		resp, err := reader.ReadString('\n')
		if err != nil {
			return err
		}
		if strings.ToLower(strings.TrimSpace(resp)) != "y" {
			fmt.Println("Aborted.")
			return nil
		}
		if err := st.DeleteVehicle(ctx, carID); err != nil {
			return err
		}
		fmt.Printf("Deleted data for %s\n", carID)
		return nil
	},
}

func init() {
	rootCmd.AddCommand(resetCmd)
	resetCmd.Flags().StringP("car", "c", "", "Vehicle ID")
}
