package api

import (
	"bytes"
	"fmt"
	htmltemplate "html/template"
	"net/url"
	"strings"
	texttemplate "text/template"

	"github.com/jackiabishop/mileminder/internal/notify"
)

var resetTextTemplate = texttemplate.Must(texttemplate.New("reset-text").Parse(`Use this link to reset your MileMinder password:

{{.Link}}

This link expires in 1 hour. If you did not request a password reset, you can ignore this email.
`))

var resetHTMLTemplate = htmltemplate.Must(htmltemplate.New("reset-html").Parse(`<p>Use this link to reset your MileMinder password:</p>
<p><a href="{{.Link}}">Reset your password</a></p>
<p>This link expires in 1 hour. If you did not request a password reset, you can ignore this email.</p>`))

type resetEmailData struct {
	Link string
}

func renderPasswordResetMessage(baseURL, token string) (notify.Message, error) {
	data := resetEmailData{Link: passwordResetURL(baseURL, token)}

	var text bytes.Buffer
	if err := resetTextTemplate.Execute(&text, data); err != nil {
		return notify.Message{}, fmt.Errorf("render reset text: %w", err)
	}
	var html bytes.Buffer
	if err := resetHTMLTemplate.Execute(&html, data); err != nil {
		return notify.Message{}, fmt.Errorf("render reset html: %w", err)
	}
	return notify.Message{
		Subject: "Reset your MileMinder password",
		Body:    strings.TrimSpace(text.String()),
		HTML:    strings.TrimSpace(html.String()),
	}, nil
}

func passwordResetURL(baseURL, token string) string {
	baseURL = strings.TrimSpace(baseURL)
	if baseURL == "" {
		baseURL = "http://localhost:8080"
	}
	u, err := url.Parse(baseURL)
	if err == nil && u.Scheme != "" && u.Host != "" {
		u.Path = strings.TrimRight(u.Path, "/") + "/reset"
		u.RawQuery = url.Values{"token": []string{token}}.Encode()
		u.Fragment = ""
		return u.String()
	}
	return strings.TrimRight(baseURL, "/") + "/reset?token=" + url.QueryEscape(token)
}
