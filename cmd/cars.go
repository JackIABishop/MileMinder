/*
Copyright © 2025 NAME HERE <EMAIL ADDRESS>
*/
package cmd

import (
	"fmt"

	"github.com/spf13/cobra"
)

var carsCmd = &cobra.Command{
	Use:   "cars",
	Short: "List all vehicles",
	RunE: func(cmd *cobra.Command, args []string) error {
		st, err := openStore()
		if err != nil {
			return err
		}
		ctx := cmd.Context()

		records, err := st.ListVehicles(ctx)
		if err != nil {
			return err
		}
		if len(records) == 0 {
			fmt.Println("No vehicles found. Have you run `mileminder init`?")
			return nil
		}

		defaultID, err := st.GetCurrent(ctx)
		if err != nil {
			return err
		}

		for _, r := range records {
			if r.ID == defaultID {
				fmt.Printf("* %s (default)\n", r.ID)
			} else {
				fmt.Printf("  %s\n", r.ID)
			}
		}
		return nil
	},
}

func init() {
	rootCmd.AddCommand(carsCmd)
}
