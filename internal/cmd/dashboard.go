package cmd

import (
	"fmt"
	"net"
	"net/http"
	"os/exec"
	"runtime"
	"time"

	"github.com/spf13/cobra"
	"github.com/steveyegge/gastown/internal/web"
	"github.com/steveyegge/gastown/internal/workspace"
)

var dashboardNoAuth bool

var (
	dashboardPort int
	dashboardOpen bool
	dashboardBind string
)

var dashboardCmd = &cobra.Command{
	Use:     "dashboard",
	GroupID: GroupDiag,
	Short:   "Start the convoy tracking web dashboard",
	Long: `Start a web server that displays the convoy tracking dashboard.

The dashboard shows real-time convoy status with:
- Convoy list with status indicators
- Progress tracking for each convoy
- Last activity indicator (green/yellow/red)
- Auto-refresh every 30 seconds via htmx

Example:
  gt dashboard              # Start on default port 8080
  gt dashboard --port 3000  # Start on port 3000
  gt dashboard --open       # Start and open browser`,
	RunE: runDashboard,
}

func init() {
	dashboardCmd.Flags().IntVar(&dashboardPort, "port", 8080, "HTTP port to listen on")
	dashboardCmd.Flags().StringVar(&dashboardBind, "bind", "0.0.0.0", "Address to bind to (0.0.0.0 for all interfaces)")
	dashboardCmd.Flags().BoolVar(&dashboardOpen, "open", false, "Open browser automatically")
	dashboardCmd.Flags().BoolVar(&dashboardNoAuth, "no-auth", false, "Disable authentication (use with caution)")
	rootCmd.AddCommand(dashboardCmd)
}

func runDashboard(cmd *cobra.Command, args []string) error {
	// Verify we're in a workspace
	townRoot, err := workspace.FindFromCwdOrError()
	if err != nil {
		return fmt.Errorf("not in a Gas Town workspace: %w", err)
	}

	// Create the live convoy fetcher
	fetcher, err := web.NewLiveConvoyFetcher()
	if err != nil {
		return fmt.Errorf("creating convoy fetcher: %w", err)
	}

	// Create the convoy handler
	convoyHandler, err := web.NewConvoyHandler(fetcher)
	if err != nil {
		return fmt.Errorf("creating convoy handler: %w", err)
	}

	// Determine the final handler based on auth mode
	var handler http.Handler

	if dashboardNoAuth {
		// No authentication - serve convoy handler directly
		fmt.Println("⚠️  Authentication disabled (--no-auth)")
		handler = convoyHandler
	} else {
		// Create auth handler and register convoy handler as protected
		authHandler, err := web.NewAuthHandler(townRoot)
		if err != nil {
			return fmt.Errorf("creating auth handler: %w", err)
		}

		// Register the convoy handler under the root path
		authHandler.RegisterProtected("/", convoyHandler)
		handler = authHandler

		if authHandler.IsEnabled() {
			fmt.Println("🔐 Authentication enabled")
		} else {
			fmt.Println("🔐 Authentication not configured - will prompt for setup")
		}
	}

	// Build the bind address
	addr := fmt.Sprintf("%s:%d", dashboardBind, dashboardPort)
	localURL := fmt.Sprintf("http://localhost:%d", dashboardPort)

	// Open browser if requested
	if dashboardOpen {
		go openBrowser(localURL)
	}

	// Start the server with timeouts
	fmt.Printf("🚚 Gas Town Dashboard starting on %s\n", addr)
	fmt.Printf("   Local:   %s\n", localURL)

	// Show LAN IP for WSL/remote access
	if lanIP := getLANIP(); lanIP != "" {
		fmt.Printf("   Network: http://%s:%d\n", lanIP, dashboardPort)
	}
	fmt.Printf("   Press Ctrl+C to stop\n")

	server := &http.Server{
		Addr:              addr,
		Handler:           handler,
		ReadHeaderTimeout: 10 * time.Second,
		ReadTimeout:       30 * time.Second,
		WriteTimeout:      60 * time.Second,
		IdleTimeout:       120 * time.Second,
	}
	return server.ListenAndServe()
}

// openBrowser opens the specified URL in the default browser.
func openBrowser(url string) {
	var cmd *exec.Cmd
	switch runtime.GOOS {
	case "darwin":
		cmd = exec.Command("open", url)
	case "linux":
		cmd = exec.Command("xdg-open", url)
	case "windows":
		cmd = exec.Command("cmd", "/c", "start", url)
	default:
		return
	}
	_ = cmd.Start()
}

// getLANIP returns the first non-loopback IPv4 address, useful for WSL/LAN access.
func getLANIP() string {
	addrs, err := net.InterfaceAddrs()
	if err != nil {
		return ""
	}
	for _, addr := range addrs {
		if ipnet, ok := addr.(*net.IPNet); ok && !ipnet.IP.IsLoopback() {
			if ipnet.IP.To4() != nil {
				return ipnet.IP.String()
			}
		}
	}
	return ""
}
