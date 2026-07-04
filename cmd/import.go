package cmd

import (
	"context"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/spf13/cobra"

	"github.com/jackiabishop/mileminder/internal/readings"
	"github.com/jackiabishop/mileminder/internal/storage"
)

var importCmd = &cobra.Command{
	Use:   "import <file.csv>",
	Short: "Bulk-import odometer readings from a CSV file",
	Long: `Bulk-import historical odometer readings from a CSV file in the export
format (header "date,miles", then YYYY-MM-DD,<miles> rows), so export -> import
round-trips cleanly.

The import is all-or-nothing: any invalid row rejects the whole file with every
error reported. Dates that already have a reading are skipped unless
--overwrite is set. The combined readings must never decrease in date order
unless --force is set.`,
	Args: cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		carID, _ := cmd.Flags().GetString("car")
		if carID == "" {
			return fmt.Errorf("please provide a vehicle ID with --car")
		}
		overwrite, _ := cmd.Flags().GetBool("overwrite")
		force, _ := cmd.Flags().GetBool("force")

		f, err := os.Open(args[0])
		if err != nil {
			return fmt.Errorf("open csv: %w", err)
		}
		defer f.Close()

		st, err := openStore()
		if err != nil {
			return err
		}

		report, err := runImport(cmd.Context(), st, carID, f, overwrite, force)
		if err != nil {
			return err
		}
		fmt.Printf("Imported %d reading(s) into %s (skipped %d, overwrote %d)\n",
			report.Added, carID, report.Skipped, report.Overwritten)
		return nil
	},
}

// runImport is the CLI import pipeline: load the vehicle, parse the CSV,
// merge (skip-by-default), enforce the monotonic rule unless forced, and
// persist with a single SaveVehicle so the import is all-or-nothing. Shared
// by "import" and "init --import".
func runImport(ctx context.Context, st storage.Store, carID string, r io.Reader, overwrite, force bool) (readings.Report, error) {
	data, err := st.GetVehicle(ctx, carID)
	if err != nil {
		return readings.Report{}, err
	}

	rows, rowErrs := readings.ParseCSV(r)
	if len(rowErrs) > 0 {
		msgs := make([]string, len(rowErrs))
		for i, e := range rowErrs {
			msgs[i] = e.Error()
		}
		return readings.Report{}, fmt.Errorf("csv has %d invalid row(s); nothing imported:\n  %s",
			len(rowErrs), strings.Join(msgs, "\n  "))
	}

	merged, report := readings.Merge(data.Readings, rows, overwrite)
	if !force {
		if err := readings.CheckMonotonic(merged); err != nil {
			return readings.Report{}, fmt.Errorf("%w; use --force to override", err)
		}
	}

	data.Readings = merged
	if err := st.SaveVehicle(ctx, carID, data); err != nil {
		return readings.Report{}, fmt.Errorf("save vehicle: %w", err)
	}
	return report, nil
}

func init() {
	rootCmd.AddCommand(importCmd)
	importCmd.Flags().StringP("car", "c", "", "Vehicle ID")
	importCmd.Flags().Bool("overwrite", false, "Replace existing readings on dates the CSV also contains")
	importCmd.Flags().Bool("force", false, "Allow the combined readings to decrease over time")
}
