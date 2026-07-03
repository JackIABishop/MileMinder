package cmd

import (
	"fmt"
	"log"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"time"

	"github.com/jackiabishop/mileminder/internal/alerts"
	"github.com/jackiabishop/mileminder/internal/api"
	"github.com/jackiabishop/mileminder/internal/auth/filestore"
	"github.com/jackiabishop/mileminder/internal/notify"
	"github.com/jackiabishop/mileminder/internal/notify/smtpchannel"
	"github.com/jackiabishop/mileminder/internal/storage/yamlstore"
	"github.com/jackiabishop/mileminder/internal/web"
	"github.com/spf13/cobra"
)

var serveCmd = &cobra.Command{
	Use:   "serve",
	Short: "Start the web UI server",
	Long: `Start an HTTP server that serves the MileMinder web interface.
The web UI provides a visual dashboard for tracking mileage, adding readings,
and viewing graphs of your usage against your allowance.`,
	RunE: func(cmd *cobra.Command, args []string) error {
		port, _ := cmd.Flags().GetInt("port")
		noBrowser, _ := cmd.Flags().GetBool("no-browser")
		devMode, _ := cmd.Flags().GetBool("dev")

		addr := fmt.Sprintf(":%d", port)
		url := fmt.Sprintf("http://localhost:%d", port)

		var handler http.Handler
		var err error
		if hostedMode(cmd) {
			handler, err = hostedHandler(cmd, devMode, url)
		} else {
			handler, err = singleUserHandler(devMode, url)
		}
		if err != nil {
			return err
		}

		fmt.Printf("🚗 MileMinder Web UI starting at %s\n", url)

		// Open browser
		if !noBrowser && !devMode {
			go openBrowser(url)
		}

		return http.ListenAndServe(addr, handler)
	},
}

// singleUserHandler builds the default, no-auth handler over the local
// ~/.mileminder store — behaviour unchanged from before Phase 2.
func singleUserHandler(devMode bool, url string) (http.Handler, error) {
	store, err := openStore()
	if err != nil {
		return nil, err
	}
	if devMode {
		fmt.Println("🔧 Development mode: API only")
		fmt.Printf("   API server: %s/api/v1\n", url)
		fmt.Println("   Run 'npm run dev' in the web/ directory for the frontend")
		return api.NewRouter(store, ""), nil
	}
	staticFS := web.GetFS()
	if staticFS == nil {
		return nil, fmt.Errorf("web UI not built; run 'cd web && npm run build' first, or use --dev mode")
	}
	return api.NewRouterWithFS(store, staticFS), nil
}

// hostedHandler builds the multi-tenant handler: per-user YAML directories plus
// file-backed user/session stores under the hosted data root. Auth gates every
// data endpoint.
func hostedHandler(cmd *cobra.Command, devMode bool, url string) (http.Handler, error) {
	dataDir, err := hostedDataDir(cmd)
	if err != nil {
		return nil, err
	}
	secure, _ := cmd.Flags().GetBool("secure-cookies")
	channel, err := notificationChannel()
	if err != nil {
		return nil, err
	}
	alertPrefs := alerts.NewFilePrefsStore(dataDir)
	alertState := alerts.NewFileStateStore(dataDir)
	users := filestore.NewUserStore(dataDir)
	tenants := yamlstore.NewTenants(dataDir)
	cfg := api.HostedConfig{
		Users:         users,
		Sessions:      filestore.NewSessionStore(dataDir),
		Tenants:       tenants,
		Notifier:      channel,
		SecureCookies: secure,
	}

	fmt.Printf("🔐 Hosted (multi-user) mode — data root: %s\n", dataDir)
	if !secure {
		fmt.Println("   ⚠  secure cookies disabled (--secure-cookies=false); use only over plain-HTTP localhost")
	}
	if noAlerts, _ := cmd.Flags().GetBool("no-alerts"); noAlerts {
		fmt.Println("   alerts scheduler disabled (--no-alerts)")
	} else {
		interval, err := alertsInterval(cmd)
		if err != nil {
			return nil, err
		}
		scheduler := &alerts.Scheduler{
			Users:    users,
			Tenants:  tenants,
			State:    alertState,
			Prefs:    alertPrefs,
			Channel:  channel,
			Now:      time.Now,
			Interval: interval,
			BaseURL:  url,
			Logger:   log.Default(),
		}
		go scheduler.Run(cmd.Context())
		fmt.Printf("   alerts scheduler interval: %s\n", interval)
	}

	if devMode {
		fmt.Println("🔧 Development mode: API only")
		fmt.Printf("   API server: %s/api/v1\n", url)
		return api.NewHostedRouterDir(cfg, ""), nil
	}
	staticFS := web.GetFS()
	if staticFS == nil {
		return nil, fmt.Errorf("web UI not built; run 'cd web && npm run build' first, or use --dev mode")
	}
	return api.NewHostedRouter(cfg, staticFS), nil
}

// hostedMode is true when --hosted is set or MILEMINDER_HOSTED is truthy.
func hostedMode(cmd *cobra.Command) bool {
	if v, _ := cmd.Flags().GetBool("hosted"); v {
		return true
	}
	switch os.Getenv("MILEMINDER_HOSTED") {
	case "1", "true", "yes":
		return true
	}
	return false
}

// hostedDataDir resolves the hosted data root: --data-dir, else
// MILEMINDER_DATA_DIR, else ~/.mileminder-hosted.
func hostedDataDir(cmd *cobra.Command) (string, error) {
	if dir, _ := cmd.Flags().GetString("data-dir"); dir != "" {
		return dir, nil
	}
	if dir := os.Getenv("MILEMINDER_DATA_DIR"); dir != "" {
		return dir, nil
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("locate home directory: %w", err)
	}
	return filepath.Join(home, ".mileminder-hosted"), nil
}

func alertsInterval(cmd *cobra.Command) (time.Duration, error) {
	if !cmd.Flags().Changed("alerts-interval") {
		if raw := os.Getenv("MILEMINDER_ALERTS_INTERVAL"); raw != "" {
			d, err := time.ParseDuration(raw)
			if err != nil {
				return 0, fmt.Errorf("invalid MILEMINDER_ALERTS_INTERVAL %q: %w", raw, err)
			}
			if d <= 0 {
				return 0, fmt.Errorf("MILEMINDER_ALERTS_INTERVAL must be greater than 0")
			}
			return d, nil
		}
	}
	d, _ := cmd.Flags().GetDuration("alerts-interval")
	if d <= 0 {
		return 0, fmt.Errorf("--alerts-interval must be greater than 0")
	}
	return d, nil
}

func notificationChannel() (notify.Channel, error) {
	cfg, ok, err := smtpchannel.ConfigFromEnv()
	if err != nil {
		return nil, err
	}
	if ok {
		ch, err := smtpchannel.New(cfg)
		if err != nil {
			return nil, err
		}
		return ch, nil
	}
	fmt.Println("   ⚠  SMTP not configured; alerts will be logged instead of emailed")
	return notify.LogChannel{Logger: log.Default()}, nil
}

func openBrowser(url string) {
	var err error
	switch runtime.GOOS {
	case "darwin":
		err = exec.Command("open", url).Start()
	case "linux":
		err = exec.Command("xdg-open", url).Start()
	case "windows":
		err = exec.Command("rundll32", "url.dll,FileProtocolHandler", url).Start()
	}
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to open browser: %v\n", err)
	}
}

func init() {
	rootCmd.AddCommand(serveCmd)
	serveCmd.Flags().IntP("port", "p", 8080, "Port to run the server on")
	serveCmd.Flags().Bool("no-browser", false, "Don't open browser automatically")
	serveCmd.Flags().Bool("dev", false, "Development mode (API only, no static files)")
	serveCmd.Flags().Bool("hosted", false, "Hosted multi-user mode: require login, isolate data per user (env: MILEMINDER_HOSTED)")
	serveCmd.Flags().String("data-dir", "", "Hosted-mode data root (default ~/.mileminder-hosted; env: MILEMINDER_DATA_DIR)")
	serveCmd.Flags().Bool("secure-cookies", true, "Set the Secure flag on session cookies (disable only for plain-HTTP localhost testing)")
	serveCmd.Flags().Duration("alerts-interval", time.Hour, "Hosted alert scheduler interval (env: MILEMINDER_ALERTS_INTERVAL)")
	serveCmd.Flags().Bool("no-alerts", false, "Disable the hosted alert scheduler")
}
