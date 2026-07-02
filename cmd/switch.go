/*
Copyright © 2025 NAME HERE <EMAIL ADDRESS>
*/
package cmd

import (
	"errors"
	"fmt"

	"github.com/spf13/cobra"

	"github.com/jackiabishop/mileminder/internal/storage"
)

var switchCmd = &cobra.Command{
	Use:   "switch <vehicleID>",
	Short: "Set the default vehicle for future commands",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		vehicleID := args[0]

		st, err := openStore()
		if err != nil {
			return err
		}

		if err := st.SetCurrent(cmd.Context(), vehicleID); err != nil {
			if errors.Is(err, storage.ErrNotFound) {
				return fmt.Errorf("vehicle %q not found; have you initialized it?", vehicleID)
			}
			return fmt.Errorf("failed to set default vehicle: %w", err)
		}

		fmt.Printf("Default vehicle set to %s\n", vehicleID)
		return nil
	},
}

func init() {
	rootCmd.AddCommand(switchCmd)
}
