package cmd

import (
	"errors"
	"fmt"
	"math"
	"os"
	"strconv"

	"github.com/spf13/cobra"

	"github.com/jackiabishop/mileminder/internal/calc"
)

var errThresholdPositive = errors.New("threshold must be greater than 0")

var checkCmd = &cobra.Command{
	Use:   "check",
	Short: "Check mileage allowance breach status",
	Run: func(cmd *cobra.Command, args []string) {
		anyBreached, err := runCheck(cmd)
		if err != nil {
			fmt.Fprintf(cmd.ErrOrStderr(), "check: %v\n", err)
			os.Exit(2)
		}
		if anyBreached {
			os.Exit(1)
		}
	},
}

func runCheck(cmd *cobra.Command) (bool, error) {
	st, err := openStore()
	if err != nil {
		return false, fmt.Errorf("open store: %w", err)
	}

	ctx := cmd.Context()
	carFlag, err := cmd.Flags().GetString("car")
	if err != nil {
		return false, fmt.Errorf("read car flag: %w", err)
	}
	all, err := cmd.Flags().GetBool("all")
	if err != nil {
		return false, fmt.Errorf("read all flag: %w", err)
	}
	threshold, err := cmd.Flags().GetFloat64("threshold")
	if err != nil {
		return false, fmt.Errorf("read threshold flag: %w", err)
	}
	if threshold <= 0 {
		return false, fmt.Errorf("invalid threshold %.1f: %w", threshold, errThresholdPositive)
	}
	if all && carFlag != "" {
		return false, fmt.Errorf("invalid flags: %w", errors.New("--all and --car cannot be used together"))
	}

	if all {
		records, err := st.ListVehicles(ctx)
		if err != nil {
			return false, fmt.Errorf("list vehicles: %w", err)
		}

		anyBreached := false
		for _, r := range records {
			s := calc.ComputeStatus(r.ID, r.Data)
			if printCheckStatus(cmd, s, threshold) {
				anyBreached = true
			}
		}
		return anyBreached, nil
	}

	carID, err := defaultVehicleID(ctx, st, carFlag)
	if err != nil {
		return false, fmt.Errorf("resolve vehicle: %w", err)
	}
	data, err := st.GetVehicle(ctx, carID)
	if err != nil {
		return false, fmt.Errorf("load vehicle %q: %w", carID, err)
	}

	s := calc.ComputeStatus(carID, data)
	return printCheckStatus(cmd, s, threshold), nil
}

func breached(s calc.Status, threshold float64) bool {
	return calc.EvaluateBreach(s, threshold).Breached()
}

func printCheckStatus(cmd *cobra.Command, s calc.Status, threshold float64) bool {
	isBreached := breached(s, threshold)
	if !isBreached {
		fmt.Fprintf(cmd.OutOrStdout(), "✓ %s: OK — %.0f%% used\n", s.ID, s.PercentUsed)
		return false
	}

	line := fmt.Sprintf("⚠ %s: OVER — %.0f%% used", s.ID, s.PercentUsed)
	if s.Delta > 0 {
		line += fmt.Sprintf(", %s mi", formatSignedMiles(s.Delta))
	}
	if s.ProjectedOver {
		line += ", projected to breach"
	}
	fmt.Fprintln(cmd.OutOrStdout(), line)
	return true
}

func formatSignedMiles(v float64) string {
	sign := ""
	if v > 0 {
		sign = "+"
	} else if v < 0 {
		sign = "-"
	}
	return sign + formatMiles(absRounded(v))
}

func absRounded(v float64) int64 {
	return int64(math.Round(math.Abs(v)))
}

func formatMiles(v int64) string {
	s := strconv.FormatInt(v, 10)
	if len(s) <= 3 {
		return s
	}

	out := make([]byte, 0, len(s)+(len(s)-1)/3)
	firstGroup := len(s) % 3
	if firstGroup == 0 {
		firstGroup = 3
	}
	out = append(out, s[:firstGroup]...)
	for i := firstGroup; i < len(s); i += 3 {
		out = append(out, ',')
		out = append(out, s[i:i+3]...)
	}
	return string(out)
}

func init() {
	rootCmd.AddCommand(checkCmd)
	checkCmd.Flags().StringP("car", "c", "", "Vehicle ID")
	checkCmd.Flags().Bool("all", false, "Check every vehicle")
	checkCmd.Flags().Float64("threshold", 100, "Percent used threshold for breach")
}
