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

func TestRenderReminderMessageWithReading(t *testing.T) {
	status := calc.Status{ID: "golf", Vehicle: "Golf", LatestReading: 12345, LatestDate: "2025-04-11"}
	msg, err := RenderReminderMessage(status, 9, "https://app.example.com")
	if err != nil {
		t.Fatalf("RenderReminderMessage: %v", err)
	}
	if !strings.Contains(msg.Subject, "log a reading for Golf") {
		t.Fatalf("subject = %q", msg.Subject)
	}
	for _, want := range []string{"Golf", "9 days", "12,345 mi", "2025-04-11", "https://app.example.com/settings"} {
		if !strings.Contains(msg.Body, want) {
			t.Fatalf("body missing %q:\n%s", want, msg.Body)
		}
		if !strings.Contains(msg.HTML, want) {
			t.Fatalf("HTML missing %q:\n%s", want, msg.HTML)
		}
	}
}

func TestRenderReminderMessageNoReading(t *testing.T) {
	status := calc.Status{ID: "golf", Vehicle: "Golf"}
	msg, err := RenderReminderMessage(status, 31, "")
	if err != nil {
		t.Fatalf("RenderReminderMessage: %v", err)
	}
	if !strings.Contains(msg.Body, "haven't logged any readings for Golf") {
		t.Fatalf("body missing no-reading copy:\n%s", msg.Body)
	}
	if strings.Contains(msg.Body, "Last reading") {
		t.Fatalf("no-reading body should omit last reading:\n%s", msg.Body)
	}
}

func TestRenderReminderMessageSingularDay(t *testing.T) {
	status := calc.Status{ID: "golf", Vehicle: "Golf", LatestReading: 100, LatestDate: "2025-04-11"}
	msg, err := RenderReminderMessage(status, 1, "")
	if err != nil {
		t.Fatalf("RenderReminderMessage: %v", err)
	}
	if !strings.Contains(msg.Body, "1 day ") {
		t.Fatalf("body should say '1 day' singular:\n%s", msg.Body)
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
