package cmd

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"time"

	"github.com/guptarohit/asciigraph"
	"github.com/spf13/cobra"
	"gopkg.in/yaml.v3"

	"github.com/jackbishop/mileage-cli/internal/model"
)

var graphCmd = &cobra.Command{
	Use:   "graph",
	Short: "ASCII graph of actual vs. ideal mileage over time",
	RunE: func(cmd *cobra.Command, args []string) error {
		// Determine vehicle ID
		carID, _ := cmd.Flags().GetString("car")
		if carID == "" {
			home, err := os.UserHomeDir()
			if err != nil {
				return err
			}
			data, err := os.ReadFile(filepath.Join(home, ".mileage-cli", "current"))
			if err != nil {
				return fmt.Errorf("no vehicle specified and no default set; use --car or switch")
			}
			carID = string(data)
		}

		// Load YAML
		home, err := os.UserHomeDir()
		if err != nil {
			return err
		}
		raw, err := os.ReadFile(filepath.Join(home, ".mileage-cli", carID+".yml"))
		if err != nil {
			return err
		}
		var v model.VehicleData
		if err := yaml.Unmarshal(raw, &v); err != nil {
			return err
		}

		// Sort dates and build series
		dates := make([]string, 0, len(v.Readings))
		for d := range v.Readings {
			dates = append(dates, d)
		}
		sort.Strings(dates)

		var actuals []float64
		var ideals []float64
		start := v.Plan.Start
		annual := float64(v.Plan.AnnualAllowance)
		// Use relative usage (miles driven since plan start)
		baseMiles := float64(v.Plan.StartMiles)

		for _, ds := range dates {
			t, _ := time.Parse("2006-01-02", ds)
			miles := float64(v.Readings[ds]) - baseMiles
			actuals = append(actuals, miles)
			daysElapsed := t.Sub(start).Hours() / 24.0
			if daysElapsed < 0 {
				daysElapsed = 0
			}
			ideal := annual * daysElapsed / 365.0
			ideals = append(ideals, ideal)
		}

		// Combine into one graph, plotting actuals and ideals
		graph := asciigraph.PlotMany(
			[][]float64{actuals, ideals},
			asciigraph.SeriesColors(asciigraph.Green, asciigraph.Cyan),
			asciigraph.Width(60),
			asciigraph.Height(15),
			asciigraph.Caption(fmt.Sprintf("Mileage Usage for %s", carID)),
		)
		fmt.Println(graph)
		return nil
	},
}

func init() {
	rootCmd.AddCommand(graphCmd)
	graphCmd.Flags().StringP("car", "c", "", "Vehicle ID")
}
