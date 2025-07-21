/*
Copyright © 2025 NAME HERE <EMAIL ADDRESS>
*/
package cmd

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/spf13/cobra"
)

var switchCmd = &cobra.Command{
	Use:   "switch <vehicleID>",
	Short: "Set the default vehicle for future commands",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		vehicleID := args[0]

		home, err := os.UserHomeDir()
		if err != nil {
			return fmt.Errorf("unable to find home directory: %w", err)
		}
		dir := filepath.Join(home, ".mileminder")
		filePath := filepath.Join(dir, vehicleID+".yml")

		// Ensure the YAML file exists
		if _, err := os.Stat(filePath); os.IsNotExist(err) {
			return fmt.Errorf("vehicle %q not found; have you initialized it?", vehicleID)
		}

		// Write the current default vehicle ID
		currentFile := filepath.Join(dir, "current")
		if err := os.WriteFile(currentFile, []byte(vehicleID), 0644); err != nil {
			return fmt.Errorf("failed to write default vehicle: %w", err)
		}

		fmt.Printf("Default vehicle set to %s\n", vehicleID)
		return nil
	},
}

func init() {
	rootCmd.AddCommand(switchCmd)
}
