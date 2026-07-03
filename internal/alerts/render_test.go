package alerts

import (
	"strings"
	"testing"

	"github.com/jackiabishop/mileminder/internal/calc"
)

func TestRenderBreachMessageIncludesReasonsAndFooter(t *testing.T) {
	status := calc.Status{ID: "golf", Vehicle: "Golf", HasPlan: true, Delta: 312, PercentUsed: 112}
	msg, err := RenderBreachMessage(status, calc.Breach{Over: true}, "https://app.example.com")
	if err != nil {
		t.Fatalf("RenderBreachMessage: %v", err)
	}
	for _, want := range []string{"Golf", "312 mi", "Manage alerts in Settings", "https://app.example.com/settings"} {
		if !strings.Contains(msg.Body, want) {
			t.Fatalf("body missing %q:\n%s", want, msg.Body)
		}
		if !strings.Contains(msg.HTML, want) {
			t.Fatalf("HTML missing %q:\n%s", want, msg.HTML)
		}
	}
}

func TestRenderBreachMessageProjectedReason(t *testing.T) {
	status := calc.Status{ID: "golf", Vehicle: "Golf", HasPlan: true, PercentUsed: 75, ProjectedOver: true}
	msg, err := RenderBreachMessage(status, calc.Breach{ProjectedOver: true}, "")
	if err != nil {
		t.Fatalf("RenderBreachMessage: %v", err)
	}
	if !strings.Contains(msg.Body, "projected to exceed") {
		t.Fatalf("body missing projected reason:\n%s", msg.Body)
	}
	if !strings.Contains(msg.Body, "Manage alerts in Settings on your MileMinder dashboard") {
		t.Fatalf("body missing generic footer:\n%s", msg.Body)
	}
}
