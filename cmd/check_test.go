package cmd

import (
	"testing"

	"github.com/jackiabishop/mileminder/internal/calc"
)

func TestBreached(t *testing.T) {
	tests := []struct {
		name      string
		status    calc.Status
		threshold float64
		want      bool
	}{
		{
			name:      "ok below threshold",
			status:    calc.Status{PercentUsed: 51},
			threshold: 100,
			want:      false,
		},
		{
			name:      "positive delta breaches",
			status:    calc.Status{Delta: 1, PercentUsed: 90},
			threshold: 100,
			want:      true,
		},
		{
			name:      "percent equal to threshold breaches",
			status:    calc.Status{PercentUsed: 100},
			threshold: 100,
			want:      true,
		},
		{
			name:      "percent above threshold breaches",
			status:    calc.Status{PercentUsed: 101},
			threshold: 100,
			want:      true,
		},
		{
			name:      "projected over breaches",
			status:    calc.Status{PercentUsed: 75, ProjectedOver: true},
			threshold: 100,
			want:      true,
		},
		{
			name:      "custom threshold breaches",
			status:    calc.Status{PercentUsed: 80},
			threshold: 75,
			want:      true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := breached(tt.status, tt.threshold); got != tt.want {
				t.Fatalf("breached(%+v, %v) = %v, want %v", tt.status, tt.threshold, got, tt.want)
			}
		})
	}
}
