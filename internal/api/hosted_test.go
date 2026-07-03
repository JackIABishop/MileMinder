package api_test

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"net/http/cookiejar"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/jackiabishop/mileminder/internal/alerts"
	"github.com/jackiabishop/mileminder/internal/api"
	"github.com/jackiabishop/mileminder/internal/auth"
	"github.com/jackiabishop/mileminder/internal/storage"
)

// hostedFixture bundles a hosted test server with the stores behind it so tests
// can both drive the API and inspect state.
type hostedFixture struct {
	srv      *httptest.Server
	users    *auth.MemoryUserStore
	sessions *auth.MemorySessionStore
	tenants  *storage.MemoryTenants
	prefs    *alerts.MemoryPrefsStore
}

// newHostedServer builds a hosted server on in-memory stores. mutate can adjust
// the config (e.g. inject a counting CheckPassword or a tight rate limit). The
// auth rate limit is raised by default so ordinary multi-step tests don't trip.
func newHostedServer(t *testing.T, mutate func(*api.HostedConfig)) hostedFixture {
	t.Helper()
	f := hostedFixture{
		users:    auth.NewMemoryUserStore(),
		sessions: auth.NewMemorySessionStore(),
		tenants:  storage.NewMemoryTenants(),
		prefs:    alerts.NewMemoryPrefsStore(),
	}
	cfg := api.HostedConfig{
		Users:          f.users,
		Sessions:       f.sessions,
		Tenants:        f.tenants,
		AlertPrefs:     f.prefs,
		SecureCookies:  false, // plain-HTTP httptest
		AuthRatePerSec: 1000,
		AuthRateBurst:  1000,
	}
	if mutate != nil {
		mutate(&cfg)
	}
	f.srv = httptest.NewServer(api.NewHostedRouterDir(cfg, ""))
	t.Cleanup(f.srv.Close)
	return f
}

func newClient(t *testing.T) *http.Client {
	t.Helper()
	jar, err := cookiejar.New(nil)
	if err != nil {
		t.Fatal(err)
	}
	return &http.Client{Jar: jar}
}

// signup registers a user through client (its cookie jar captures the session).
// It returns the bearer token from the response body.
func signup(t *testing.T, srv *httptest.Server, client *http.Client, email, password string) string {
	t.Helper()
	body := strings.NewReader(`{"email":"` + email + `","password":"` + password + `"}`)
	resp, err := client.Post(srv.URL+"/api/v1/auth/signup", "application/json", body)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusCreated {
		b, _ := io.ReadAll(resp.Body)
		t.Fatalf("signup %s: want 201, got %d (%s)", email, resp.StatusCode, b)
	}
	var out struct {
		Token string `json:"token"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		t.Fatal(err)
	}
	return out.Token
}

func createVehicle(t *testing.T, srv *httptest.Server, client *http.Client, id string) {
	t.Helper()
	body := `{"id":"` + id + `","vehicle":"Golf","start_date":"2025-01-01","end_date":"2028-01-01","annual_allowance":10000,"start_miles":5000}`
	req, _ := http.NewRequest(http.MethodPost, srv.URL+"/api/v1/vehicles", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	resp, err := client.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusCreated {
		b, _ := io.ReadAll(resp.Body)
		t.Fatalf("create vehicle %s: want 201, got %d (%s)", id, resp.StatusCode, b)
	}
}

// The isolation guarantee: one user cannot see another's vehicles or default.
func TestHostedUsersAreIsolated(t *testing.T) {
	f := newHostedServer(t, nil)

	alice := newClient(t)
	signup(t, f.srv, alice, "alice@example.com", "password123")
	createVehicle(t, f.srv, alice, "golf")

	// Alice sets her default.
	setCurrent(t, f.srv, alice, "golf", http.StatusOK)

	bob := newClient(t)
	signup(t, f.srv, bob, "bob@example.com", "password123")

	// Bob's garage is empty.
	list := getVehicles(t, f.srv, bob)
	if len(list) != 0 {
		t.Fatalf("bob sees alice's vehicles: %+v", list)
	}
	// Bob cannot read Alice's vehicle by id.
	resp, err := bob.Get(f.srv.URL + "/api/v1/vehicles/golf")
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusNotFound {
		t.Fatalf("bob GET alice's vehicle: want 404, got %d", resp.StatusCode)
	}
	// Bob's default pointer did not inherit Alice's.
	if cur := getCurrent(t, f.srv, bob); cur != "" {
		t.Fatalf("bob sees alice's current pointer: %q", cur)
	}
	// Alice still sees her own vehicle.
	if list := getVehicles(t, f.srv, alice); len(list) != 1 {
		t.Fatalf("alice lost her vehicle: %+v", list)
	}
}

// /meta is reachable without a session and reports hosted mode, so the SPA can
// decide to show the login flow before anyone is authenticated.
func TestHostedMetaIsOpen(t *testing.T) {
	f := newHostedServer(t, nil)

	resp, err := http.Get(f.srv.URL + "/api/v1/meta")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("meta: want 200, got %d", resp.StatusCode)
	}
	var meta struct {
		Mode string `json:"mode"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&meta); err != nil {
		t.Fatal(err)
	}
	if meta.Mode != "hosted" {
		t.Fatalf("want mode hosted, got %q", meta.Mode)
	}
}

func TestHostedRequiresAuth(t *testing.T) {
	f := newHostedServer(t, nil)

	for _, tc := range []struct{ name, auth string }{
		{"no token", ""},
		{"garbage bearer", "Bearer not-a-real-token"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			req, _ := http.NewRequest(http.MethodGet, f.srv.URL+"/api/v1/vehicles", nil)
			if tc.auth != "" {
				req.Header.Set("Authorization", tc.auth)
			}
			resp, err := http.DefaultClient.Do(req)
			if err != nil {
				t.Fatal(err)
			}
			resp.Body.Close()
			if resp.StatusCode != http.StatusUnauthorized {
				t.Fatalf("want 401, got %d", resp.StatusCode)
			}
		})
	}
}

func TestHostedLogoutRevokesSession(t *testing.T) {
	f := newHostedServer(t, nil)
	client := newClient(t)
	token := signup(t, f.srv, client, "alice@example.com", "password123")

	// Authenticated request works.
	if list := getVehicles(t, f.srv, client); list == nil {
		t.Fatal("expected non-nil (empty) vehicle list while authenticated")
	}

	// Logout.
	req, _ := http.NewRequest(http.MethodPost, f.srv.URL+"/api/v1/auth/logout", nil)
	resp, err := client.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("logout: want 200, got %d", resp.StatusCode)
	}

	// The same token no longer authenticates (via Bearer, since the cookie is cleared).
	req, _ = http.NewRequest(http.MethodGet, f.srv.URL+"/api/v1/vehicles", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	resp, err = http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("after logout: want 401, got %d", resp.StatusCode)
	}
}

// A cookie-authenticated, state-changing request from a cross-site context is
// rejected; the same request with a Bearer token (no ambient credential) is not
// subject to the CSRF check.
func TestHostedCSRF(t *testing.T) {
	f := newHostedServer(t, nil)
	client := newClient(t)
	token := signup(t, f.srv, client, "alice@example.com", "password123")

	body := func() *bytes.Reader {
		return bytes.NewReader([]byte(`{"id":"golf","vehicle":"Golf","start_date":"2025-01-01","end_date":"2028-01-01","annual_allowance":10000,"start_miles":5000}`))
	}

	// Cookie auth + cross-site fetch metadata → 403.
	req, _ := http.NewRequest(http.MethodPost, f.srv.URL+"/api/v1/vehicles", body())
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Sec-Fetch-Site", "cross-site")
	resp, err := client.Do(req) // client carries the session cookie
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusForbidden {
		t.Fatalf("cookie cross-site mutation: want 403, got %d", resp.StatusCode)
	}

	// Bearer auth + same cross-site metadata → allowed (no ambient credential).
	req, _ = http.NewRequest(http.MethodPost, f.srv.URL+"/api/v1/vehicles", body())
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Sec-Fetch-Site", "cross-site")
	req.Header.Set("Authorization", "Bearer "+token)
	resp, err = http.DefaultClient.Do(req) // no cookie jar
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("bearer cross-site mutation: want 201, got %d", resp.StatusCode)
	}
}

// Login runs exactly one password comparison whether or not the account exists,
// and returns an identical 401 for unknown-email and wrong-password — the
// constant-time, non-enumerable contract.
func TestHostedLoginIsConstantTimeAndGeneric(t *testing.T) {
	var calls atomic.Int32
	f := newHostedServer(t, func(cfg *api.HostedConfig) {
		cfg.CheckPassword = func(hash, password string) bool {
			calls.Add(1)
			return auth.CheckPassword(hash, password)
		}
	})

	// Register a real account.
	signup(t, f.srv, newClient(t), "real@example.com", "password123")

	login := func(email, password string) (int, string) {
		body := strings.NewReader(`{"email":"` + email + `","password":"` + password + `"}`)
		resp, err := http.Post(f.srv.URL+"/api/v1/auth/login", "application/json", body)
		if err != nil {
			t.Fatal(err)
		}
		defer resp.Body.Close()
		b, _ := io.ReadAll(resp.Body)
		return resp.StatusCode, string(b)
	}

	calls.Store(0)
	unknownStatus, unknownBody := login("nobody@example.com", "password123")
	if got := calls.Load(); got != 1 {
		t.Fatalf("unknown email: want exactly 1 password comparison, got %d", got)
	}

	calls.Store(0)
	wrongStatus, wrongBody := login("real@example.com", "wrongpassword")
	if got := calls.Load(); got != 1 {
		t.Fatalf("wrong password: want exactly 1 password comparison, got %d", got)
	}

	if unknownStatus != http.StatusUnauthorized || wrongStatus != http.StatusUnauthorized {
		t.Fatalf("want both 401, got unknown=%d wrong=%d", unknownStatus, wrongStatus)
	}
	if unknownBody != wrongBody {
		t.Fatalf("401 bodies differ, leaking account existence:\n unknown=%q\n wrong=%q", unknownBody, wrongBody)
	}
}

func TestHostedSignupValidation(t *testing.T) {
	f := newHostedServer(t, nil)

	cases := []struct {
		name, email, password string
		want                  int
	}{
		{"short password", "a@b.com", "short", http.StatusBadRequest},
		{"empty email", "", "password123", http.StatusBadRequest},
		{"no at", "nope", "password123", http.StatusBadRequest},
		{"no local part", "@x.com", "password123", http.StatusBadRequest},
		{"no domain", "a@", "password123", http.StatusBadRequest},
		{"valid", "good@example.com", "password123", http.StatusCreated},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			body := strings.NewReader(`{"email":"` + tc.email + `","password":"` + tc.password + `"}`)
			resp, err := http.Post(f.srv.URL+"/api/v1/auth/signup", "application/json", body)
			if err != nil {
				t.Fatal(err)
			}
			resp.Body.Close()
			if resp.StatusCode != tc.want {
				t.Fatalf("%s: want %d, got %d", tc.name, tc.want, resp.StatusCode)
			}
		})
	}
}

func TestHostedDuplicateSignupConflicts(t *testing.T) {
	f := newHostedServer(t, nil)
	signup(t, f.srv, newClient(t), "dup@example.com", "password123")

	body := strings.NewReader(`{"email":"DUP@example.com","password":"password123"}`)
	resp, err := http.Post(f.srv.URL+"/api/v1/auth/signup", "application/json", body)
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusConflict {
		t.Fatalf("duplicate signup: want 409, got %d", resp.StatusCode)
	}
}

func TestHostedRateLimitsAuth(t *testing.T) {
	f := newHostedServer(t, func(cfg *api.HostedConfig) {
		cfg.AuthRatePerSec = 0.01 // effectively no refill during the test
		cfg.AuthRateBurst = 3
	})

	got429 := false
	for i := 0; i < 10; i++ {
		body := strings.NewReader(`{"email":"x@example.com","password":"wrongpassword"}`)
		resp, err := http.Post(f.srv.URL+"/api/v1/auth/login", "application/json", body)
		if err != nil {
			t.Fatal(err)
		}
		resp.Body.Close()
		if resp.StatusCode == http.StatusTooManyRequests {
			got429 = true
			break
		}
	}
	if !got429 {
		t.Fatal("expected a 429 after exhausting the auth rate-limit burst")
	}
}

func TestHostedAlertPrefsDefaultAndUpdate(t *testing.T) {
	f := newHostedServer(t, nil)
	client := newClient(t)
	signup(t, f.srv, client, "alice@example.com", "password123")

	resp, err := client.Get(f.srv.URL + "/api/v1/alerts/prefs")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("get prefs: want 200, got %d", resp.StatusCode)
	}
	var got alerts.Prefs
	if err := json.NewDecoder(resp.Body).Decode(&got); err != nil {
		t.Fatal(err)
	}
	if !got.Enabled || got.Threshold != 100 {
		t.Fatalf("default prefs = %+v, want enabled threshold 100", got)
	}

	req, _ := http.NewRequest(http.MethodPut, f.srv.URL+"/api/v1/alerts/prefs", strings.NewReader(`{"enabled":false,"threshold":85}`))
	req.Header.Set("Content-Type", "application/json")
	resp, err = client.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		b, _ := io.ReadAll(resp.Body)
		t.Fatalf("put prefs: want 200, got %d (%s)", resp.StatusCode, b)
	}
	var updated alerts.Prefs
	if err := json.NewDecoder(resp.Body).Decode(&updated); err != nil {
		t.Fatal(err)
	}
	if updated.Enabled || updated.Threshold != 85 {
		t.Fatalf("updated prefs = %+v, want disabled threshold 85", updated)
	}
}

func TestHostedAlertPrefsValidationAndAuth(t *testing.T) {
	f := newHostedServer(t, nil)

	resp, err := http.Get(f.srv.URL + "/api/v1/alerts/prefs")
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("unauthenticated get prefs: want 401, got %d", resp.StatusCode)
	}

	client := newClient(t)
	signup(t, f.srv, client, "alice@example.com", "password123")
	req, _ := http.NewRequest(http.MethodPut, f.srv.URL+"/api/v1/alerts/prefs", strings.NewReader(`{"threshold":0}`))
	req.Header.Set("Content-Type", "application/json")
	resp, err = client.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("invalid threshold: want 400, got %d", resp.StatusCode)
	}
}

// --- small typed helpers over the authenticated data API ---

func getVehicles(t *testing.T, srv *httptest.Server, client *http.Client) []api.VehicleListItem {
	t.Helper()
	resp, err := client.Get(srv.URL + "/api/v1/vehicles")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("list vehicles: want 200, got %d", resp.StatusCode)
	}
	var list []api.VehicleListItem
	if err := json.NewDecoder(resp.Body).Decode(&list); err != nil {
		t.Fatal(err)
	}
	return list
}

func setCurrent(t *testing.T, srv *httptest.Server, client *http.Client, id string, want int) {
	t.Helper()
	req, _ := http.NewRequest(http.MethodPut, srv.URL+"/api/v1/current", strings.NewReader(`{"id":"`+id+`"}`))
	req.Header.Set("Content-Type", "application/json")
	resp, err := client.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if resp.StatusCode != want {
		t.Fatalf("set current: want %d, got %d", want, resp.StatusCode)
	}
}

func getCurrent(t *testing.T, srv *httptest.Server, client *http.Client) string {
	t.Helper()
	resp, err := client.Get(srv.URL + "/api/v1/current")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	var out struct {
		Current string `json:"current"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		t.Fatal(err)
	}
	return out.Current
}
