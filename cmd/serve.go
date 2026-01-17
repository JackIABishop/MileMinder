package cmd

import (
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"runtime"

	"github.com/jackiabishop/mileminder/internal/api"
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

		if devMode {
			// In dev mode, only serve API - frontend runs separately via npm run dev
			fmt.Println("🔧 Development mode: API only")
			fmt.Printf("   API server: %s/api\n", url)
			fmt.Println("   Run 'npm run dev' in the web/ directory for the frontend")
			handler = api.NewRouter("")
		} else {
			// Production mode: serve embedded files
			staticFS := web.GetFS()
			if staticFS == nil {
				return fmt.Errorf("web UI not built; run 'cd web && npm run build' first, or use --dev mode")
			}
			handler = api.NewRouterWithFS(staticFS)
		}

		fmt.Printf("🚗 MileMinder Web UI starting at %s\n", url)

		// Open browser
		if !noBrowser && !devMode {
			go openBrowser(url)
		}

		return http.ListenAndServe(addr, handler)
	},
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
}
