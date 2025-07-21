/*
Copyright © 2025 NAME HERE <EMAIL ADDRESS>
*/
package cmd

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"
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
		home, err := os.UserHomeDir()
		if err != nil {
			return err
		}
		filePath := filepath.Join(home, ".mileage-cli", carID+".yml")
		if _, err := os.Stat(filePath); os.IsNotExist(err) {
			fmt.Printf("Vehicle %q not found; have you run `mileage init`?\n", carID)
			return nil
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
		if err := os.Remove(filePath); err != nil {
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
