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
)

var carsCmd = &cobra.Command{
	Use:   "cars",
	Short: "List all vehicles",
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

		// Read default vehicle
		defaultFile := filepath.Join(dir, "current")
		defaultID := ""
		if data, err := os.ReadFile(defaultFile); err == nil {
			defaultID = strings.TrimSpace(string(data))
		}

		// Collect vehicle IDs
		var vehicles []string
		for _, e := range entries {
			if e.IsDir() {
				continue
			}
			name := e.Name()
			if filepath.Ext(name) != ".yml" {
				continue
			}
			id := strings.TrimSuffix(name, ".yml")
			vehicles = append(vehicles, id)
		}

		if len(vehicles) == 0 {
			fmt.Println("No vehicles found. Have you run `mileminder init`?")
			return nil
		}

		// Print list with default marker
		for _, v := range vehicles {
			if v == defaultID {
				fmt.Printf("* %s (default)\n", v)
			} else {
				fmt.Printf("  %s\n", v)
			}
		}
		return nil
	},
}

func init() {
	rootCmd.AddCommand(carsCmd)
}
