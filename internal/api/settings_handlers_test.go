package api_test

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"testing"

	"github.com/jackiabishop/mileminder/internal/model"
)

func getSettings(t *testing.T, url string) model.Settings {
	t.Helper()
	resp, err := http.Get(url + "/api/v1/settings")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("GET /settings: status %d", resp.StatusCode)
	}
	var got model.Settings
	if err := json.NewDecoder(resp.Body).Decode(&got); err != nil {
		t.Fatal(err)
	}
	return got
}

func putSettings(t *testing.T, url, body string) *http.Response {
	t.Helper()
	req, err := http.NewRequest(http.MethodPut, url+"/api/v1/settings", bytes.NewBufferString(body))
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { resp.Body.Close() })
	return resp
}

func TestGetSettingsDefaults(t *testing.T) {
	srv, _ := newTestServer(t, nil)

	got := getSettings(t, srv.URL)
	if want := model.DefaultSettings(); got != want {
		t.Fatalf("defaults: want %+v, got %+v", want, got)
	}
}

func TestPutSettingsRoundTrip(t *testing.T) {
	srv, st := newTestServer(t, nil)

	resp := putSettings(t, srv.URL, `{"currency":"eur"}`)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("PUT /settings: status %d", resp.StatusCode)
	}
	var echoed model.Settings
	if err := json.NewDecoder(resp.Body).Decode(&echoed); err != nil {
		t.Fatal(err)
	}
	if echoed.Currency != "EUR" {
		t.Fatalf("currency not normalised in echo: %+v", echoed)
	}
	if echoed.DistanceUnit != "mi" {
		t.Fatalf("distance unit clobbered by partial update: %+v", echoed)
	}

	// Persisted through the store, and served back on GET.
	stored, err := st.GetSettings(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if stored.Currency != "EUR" {
		t.Fatalf("currency not persisted: %+v", stored)
	}
	if got := getSettings(t, srv.URL); got.Currency != "EUR" {
		t.Fatalf("GET after PUT: %+v", got)
	}
}

func TestPutSettingsInvalidCurrency(t *testing.T) {
	srv, _ := newTestServer(t, nil)

	resp := putSettings(t, srv.URL, `{"currency":"XYZ"}`)
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("invalid currency: want 400, got %d", resp.StatusCode)
	}
	var errResp struct {
		Error struct {
			Code string `json:"code"`
		} `json:"error"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&errResp); err != nil {
		t.Fatal(err)
	}
	if errResp.Error.Code != "invalid_currency" {
		t.Fatalf("want code invalid_currency, got %q", errResp.Error.Code)
	}
}

func TestPutSettingsRejectsKm(t *testing.T) {
	srv, _ := newTestServer(t, nil)

	resp := putSettings(t, srv.URL, `{"distance_unit":"km"}`)
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("km: want 400, got %d", resp.StatusCode)
	}
	if got := getSettings(t, srv.URL); got.DistanceUnit != "mi" {
		t.Fatalf("rejected PUT must not persist: %+v", got)
	}
}
