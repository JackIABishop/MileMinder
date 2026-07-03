package cmd

import (
	"fmt"
	"sort"
	"time"

	"github.com/guptarohit/asciigraph"
	"github.com/spf13/cobra"

	"github.com/jackiabishop/mileminder/internal/calc"
)

var graphCmd = &cobra.Command{
	Use:   "graph",
	Short: "ASCII graph of actual vs. ideal mileminder over time",
	RunE: func(cmd *cobra.Command, args []string) error {
		st, err := openStore()
		if err != nil {
			return err
		}
		ctx := cmd.Context()

		// Determine vehicle ID (flag or stored default).
		carFlag, _ := cmd.Flags().GetString("car")
		carID, err := defaultVehicleID(ctx, st, carFlag)
		if err != nil {
			return err
		}

		v, err := st.GetVehicle(ctx, carID)
		if err != nil {
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
		// Use relative usage (miles driven since plan start)
		baseMiles := 0.0
		if v.Plan != nil {
			baseMiles = float64(v.Plan.StartMiles)
		} else if len(dates) > 0 {
			baseMiles = float64(v.Readings[dates[0]])
		}

		for _, ds := range dates {
			t, _ := time.Parse("2006-01-02", ds)
			miles := float64(v.Readings[ds]) - baseMiles
			actuals = append(actuals, miles)
			if v.Plan != nil {
				ideals = append(ideals, calc.AllowanceMiles(v.Plan.AnnualAllowance, v.Plan.Start, t))
			}
		}

		if v.Plan == nil {
			graph := asciigraph.Plot(
				actuals,
				asciigraph.SeriesColors(asciigraph.Green),
				asciigraph.Width(60),
				asciigraph.Height(15),
				asciigraph.Caption(fmt.Sprintf("Mileage Tracking for %s", carID)),
			)
			fmt.Println(graph)
			return nil
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
