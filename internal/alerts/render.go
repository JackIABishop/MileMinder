package alerts

import (
	"bytes"
	"fmt"
	htmltemplate "html/template"
	"math"
	"net/url"
	"strings"
	texttemplate "text/template"

	"github.com/jackiabishop/mileminder/internal/calc"
	"github.com/jackiabishop/mileminder/internal/notify"
)

var alertTextTemplate = texttemplate.Must(texttemplate.New("alert-text").Parse(`{{.Vehicle}} has crossed a MileMinder allowance alert.

{{.Reason}}

Current status:
- {{printf "%.0f" .PercentUsed}}% used
- {{.DeltaText}} against today's allowance line
{{if .ProjectedOver}}- Projected to exceed the current allowance year{{end}}

{{.Footer}}
`))

var alertHTMLTemplate = htmltemplate.Must(htmltemplate.New("alert-html").Parse(`<p>{{.Vehicle}} has crossed a MileMinder allowance alert.</p>
<p>{{.Reason}}</p>
<ul>
<li>{{printf "%.0f" .PercentUsed}}% used</li>
<li>{{.DeltaText}} against today's allowance line</li>
{{if .ProjectedOver}}<li>Projected to exceed the current allowance year</li>{{end}}
</ul>
<p>{{.Footer}}</p>`))

type alertTemplateData struct {
	Vehicle       string
	Reason        string
	PercentUsed   float64
	DeltaText     string
	ProjectedOver bool
	Footer        string
}

// RenderBreachMessage turns a breach event into a channel-neutral message.
func RenderBreachMessage(s calc.Status, b calc.Breach, baseURL string) (notify.Message, error) {
	data := alertTemplateData{
		Vehicle:       displayVehicle(s),
		Reason:        breachReason(s, b),
		PercentUsed:   s.PercentUsed,
		DeltaText:     formatSignedMiles(s.Delta),
		ProjectedOver: b.ProjectedOver,
		Footer:        footer(baseURL),
	}

	var text bytes.Buffer
	if err := alertTextTemplate.Execute(&text, data); err != nil {
		return notify.Message{}, err
	}
	var html bytes.Buffer
	if err := alertHTMLTemplate.Execute(&html, data); err != nil {
		return notify.Message{}, err
	}
	return notify.Message{
		Subject: fmt.Sprintf("Mileage alert: %s", data.Vehicle),
		Body:    strings.TrimSpace(text.String()),
		HTML:    strings.TrimSpace(html.String()),
	}, nil
}

func displayVehicle(s calc.Status) string {
	if s.Vehicle != "" {
		return s.Vehicle
	}
	return s.ID
}

func breachReason(s calc.Status, b calc.Breach) string {
	switch {
	case b.Over:
		return fmt.Sprintf("You are over today's allowance line by %s mi.", formatMiles(math.Round(math.Abs(s.Delta))))
	case b.ThresholdHit:
		return fmt.Sprintf("You have reached %.0f%% of the mileage allowance expected by today.", s.PercentUsed)
	case b.ProjectedOver:
		return "Your current pace is projected to exceed the current allowance year."
	default:
		return "Your vehicle has crossed an alert threshold."
	}
}

func footer(baseURL string) string {
	baseURL = strings.TrimSpace(baseURL)
	if baseURL == "" {
		return "Manage alerts in Settings on your MileMinder dashboard."
	}
	u, err := url.Parse(baseURL)
	if err == nil {
		u.Path = strings.TrimRight(u.Path, "/") + "/settings"
		u.RawQuery = ""
		u.Fragment = ""
		return "Manage alerts in Settings: " + u.String()
	}
	return "Manage alerts in Settings on your MileMinder dashboard."
}

func formatSignedMiles(v float64) string {
	sign := ""
	if v > 0 {
		sign = "+"
	} else if v < 0 {
		sign = "-"
	}
	return sign + formatMiles(math.Round(math.Abs(v))) + " mi"
}

func formatMiles(v float64) string {
	n := int64(v)
	s := fmt.Sprintf("%d", n)
	if len(s) <= 3 {
		return s
	}
	first := len(s) % 3
	if first == 0 {
		first = 3
	}
	var b strings.Builder
	b.WriteString(s[:first])
	for i := first; i < len(s); i += 3 {
		b.WriteByte(',')
		b.WriteString(s[i : i+3])
	}
	return b.String()
}
