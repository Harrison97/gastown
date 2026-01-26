package cmd

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/spf13/cobra"
	"github.com/steveyegge/gastown/internal/config"
	"github.com/steveyegge/gastown/internal/daemon"
	"github.com/steveyegge/gastown/internal/deacon"
	"github.com/steveyegge/gastown/internal/events"
	"github.com/steveyegge/gastown/internal/git"
	"github.com/steveyegge/gastown/internal/mayor"
	"github.com/steveyegge/gastown/internal/polecat"
	"github.com/steveyegge/gastown/internal/rig"
	"github.com/steveyegge/gastown/internal/session"
	"github.com/steveyegge/gastown/internal/style"
	"github.com/steveyegge/gastown/internal/tmux"
	"github.com/steveyegge/gastown/internal/workspace"
)

var reloadCmd = &cobra.Command{
	Use:     "reload",
	GroupID: GroupServices,
	Short:   "Reload all Gas Town services (restart with fresh binary)",
	Long: `Reload all Gas Town services by stopping and restarting them.

This is useful after installing a new version of gt/bd to ensure all
services pick up the new binary. It performs:

  1. Stop all rig agents (refineries, witnesses)
  2. Stop bd daemons (all workspaces)
  3. Restart bd daemons
  4. Start all rig agents

By default, the Mayor session is preserved (not restarted). Use --mayor
to also reload the Mayor (will kill your current session if you're in it).

Use --polecats to also stop and restart polecats with pinned work.`,
	RunE: runReload,
}

var (
	reloadQuiet    bool
	reloadMayor    bool
	reloadPolecats bool
	reloadForce    bool
)

func init() {
	reloadCmd.Flags().BoolVarP(&reloadQuiet, "quiet", "q", false, "Only show errors")
	reloadCmd.Flags().BoolVar(&reloadMayor, "mayor", false, "Also reload Mayor session (kills current session if attached)")
	reloadCmd.Flags().BoolVarP(&reloadPolecats, "polecats", "p", false, "Also reload polecats with pinned work")
	reloadCmd.Flags().BoolVarP(&reloadForce, "force", "f", false, "Force kill without graceful shutdown")
	rootCmd.AddCommand(reloadCmd)
}

func runReload(cmd *cobra.Command, args []string) error {
	townRoot, err := workspace.FindFromCwdOrError()
	if err != nil {
		return fmt.Errorf("not in a Gas Town workspace: %w", err)
	}

	t := tmux.NewTmux()
	if !t.IsAvailable() {
		return fmt.Errorf("tmux not available")
	}

	rigs := discoverRigs(townRoot)
	allOK := true

	fmt.Println("═══ Stopping services ═══")
	fmt.Println()

	// Phase 1: Stop polecats if requested
	if reloadPolecats {
		polecatsStopped := reloadStopPolecats(t, townRoot, rigs)
		if polecatsStopped > 0 {
			printReloadStatus("Polecats", true, fmt.Sprintf("%d stopped", polecatsStopped))
		} else {
			printReloadStatus("Polecats", true, "none running")
		}
	}

	// Phase 2: Stop rig agents (refineries, witnesses)
	for _, rigName := range rigs {
		// Stop refinery
		sessionName := fmt.Sprintf("gt-%s-refinery", rigName)
		wasRunning, err := reloadStopSession(t, sessionName)
		if err != nil {
			printReloadStatus(fmt.Sprintf("Refinery (%s)", rigName), false, err.Error())
			allOK = false
		} else if wasRunning {
			printReloadStatus(fmt.Sprintf("Refinery (%s)", rigName), true, "stopped")
		}

		// Stop witness
		sessionName = fmt.Sprintf("gt-%s-witness", rigName)
		wasRunning, err = reloadStopSession(t, sessionName)
		if err != nil {
			printReloadStatus(fmt.Sprintf("Witness (%s)", rigName), false, err.Error())
			allOK = false
		} else if wasRunning {
			printReloadStatus(fmt.Sprintf("Witness (%s)", rigName), true, "stopped")
		}
	}

	// Phase 3: Stop town-level sessions (Deacon, Boot, optionally Mayor)
	for _, ts := range session.TownSessions() {
		// Skip Mayor unless --mayor flag
		if ts.Name == "Mayor" && !reloadMayor {
			continue
		}
		stopped, err := session.StopTownSession(t, ts, reloadForce)
		if err != nil {
			printReloadStatus(ts.Name, false, err.Error())
			allOK = false
		} else if stopped {
			printReloadStatus(ts.Name, true, "stopped")
		}
	}

	// Phase 4: Stop gt daemon
	running, pid, _ := daemon.IsRunning(townRoot)
	if running {
		if err := daemon.StopDaemon(townRoot); err != nil {
			printReloadStatus("Daemon", false, err.Error())
			allOK = false
		} else {
			printReloadStatus("Daemon", true, fmt.Sprintf("stopped (was PID %d)", pid))
		}
	}

	// Phase 5: Stop all bd daemons (errors are non-fatal - daemon may already be stopped)
	bdWorkspaces := findBdWorkspaces(townRoot)
	for _, ws := range bdWorkspaces {
		if err := stopBdDaemon(ws); err != nil {
			// Non-fatal - daemon may already be stopped
			printReloadStatus(fmt.Sprintf("bd daemon (%s)", shortPath(ws)), true, "stopped (was not running)")
		} else {
			printReloadStatus(fmt.Sprintf("bd daemon (%s)", shortPath(ws)), true, "stopped")
		}
	}

	fmt.Println()
	fmt.Println("═══ Starting services ═══")
	fmt.Println()

	// Phase 6: Start bd daemons
	for _, ws := range bdWorkspaces {
		if err := startBdDaemon(ws); err != nil {
			printReloadStatus(fmt.Sprintf("bd daemon (%s)", shortPath(ws)), false, err.Error())
			allOK = false
		} else {
			printReloadStatus(fmt.Sprintf("bd daemon (%s)", shortPath(ws)), true, "started")
		}
	}

	// Phase 7: Start gt daemon
	if err := ensureDaemon(townRoot); err != nil {
		printReloadStatus("Daemon", false, err.Error())
		allOK = false
	} else {
		running, pid, _ := daemon.IsRunning(townRoot)
		if running {
			printReloadStatus("Daemon", true, fmt.Sprintf("PID %d", pid))
		}
	}

	// Phase 8: Start town-level sessions (Deacon, optionally Mayor)
	if reloadMayor {
		mayorMgr := mayor.NewManager(townRoot)
		if err := mayorMgr.Start(""); err != nil && err != mayor.ErrAlreadyRunning {
			printReloadStatus("Mayor", false, err.Error())
			allOK = false
		} else {
			printReloadStatus("Mayor", true, mayorMgr.SessionName())
		}
	}

	deaconMgr := deacon.NewManager(townRoot)
	if err := deaconMgr.Start(""); err != nil && err != deacon.ErrAlreadyRunning {
		printReloadStatus("Deacon", false, err.Error())
		allOK = false
	} else {
		printReloadStatus("Deacon", true, deaconMgr.SessionName())
	}

	// Phase 9: Start rig agents in parallel
	prefetchedRigs, rigErrors := prefetchRigs(rigs)
	witnessResults, refineryResults := startRigAgentsWithPrefetch(rigs, prefetchedRigs, rigErrors)

	for _, rigName := range rigs {
		if result, ok := witnessResults[rigName]; ok {
			printReloadStatus(result.name, result.ok, result.detail)
			if !result.ok {
				allOK = false
			}
		}
	}
	for _, rigName := range rigs {
		if result, ok := refineryResults[rigName]; ok {
			printReloadStatus(result.name, result.ok, result.detail)
			if !result.ok {
				allOK = false
			}
		}
	}

	// Phase 10: Start polecats with work if requested
	if reloadPolecats {
		for _, rigName := range rigs {
			polecatsStarted, polecatErrors := startPolecatsWithWork(townRoot, rigName)
			for _, name := range polecatsStarted {
				printReloadStatus(fmt.Sprintf("Polecat (%s/%s)", rigName, name), true, "started")
			}
			for name, err := range polecatErrors {
				printReloadStatus(fmt.Sprintf("Polecat (%s/%s)", rigName, name), false, err.Error())
				allOK = false
			}
		}
	}

	// Summary
	fmt.Println()
	if allOK {
		fmt.Printf("%s All services reloaded\n", style.Bold.Render("✓"))
		_ = events.LogFeed(events.TypeBoot, "gt", events.BootPayload("reload", []string{"all"}))
	} else {
		fmt.Printf("%s Some services failed to reload\n", style.Bold.Render("✗"))
		return fmt.Errorf("not all services reloaded")
	}

	return nil
}

func printReloadStatus(name string, ok bool, detail string) {
	if reloadQuiet && ok {
		return
	}
	if ok {
		fmt.Printf("%s %s: %s\n", style.SuccessPrefix, name, style.Dim.Render(detail))
	} else {
		fmt.Printf("%s %s: %s\n", style.ErrorPrefix, name, detail)
	}
}

// reloadStopSession gracefully stops a tmux session.
func reloadStopSession(t *tmux.Tmux, sessionName string) (bool, error) {
	running, err := t.HasSession(sessionName)
	if err != nil {
		return false, err
	}
	if !running {
		return false, nil
	}

	if !reloadForce {
		_ = t.SendKeysRaw(sessionName, "C-c")
		time.Sleep(100 * time.Millisecond)
	}

	return true, t.KillSessionWithProcesses(sessionName)
}

// reloadStopPolecats stops all polecat sessions across all rigs.
func reloadStopPolecats(t *tmux.Tmux, townRoot string, rigNames []string) int {
	stopped := 0

	rigsConfigPath := filepath.Join(townRoot, "mayor", "rigs.json")
	rigsConfig, err := config.LoadRigsConfig(rigsConfigPath)
	if err != nil {
		rigsConfig = &config.RigsConfig{Rigs: make(map[string]config.RigEntry)}
	}

	g := git.NewGit(townRoot)
	rigMgr := rig.NewManager(townRoot, rigsConfig, g)

	for _, rigName := range rigNames {
		r, err := rigMgr.GetRig(rigName)
		if err != nil {
			continue
		}

		polecatMgr := polecat.NewSessionManager(t, r)
		infos, err := polecatMgr.List()
		if err != nil {
			continue
		}

		for _, info := range infos {
			if err := polecatMgr.Stop(info.Polecat, reloadForce); err == nil {
				stopped++
			}
		}
	}

	return stopped
}

// findBdWorkspaces finds all bd workspace directories that might have daemons.
func findBdWorkspaces(townRoot string) []string {
	workspaces := []string{}

	// Main town workspace
	if _, err := os.Stat(filepath.Join(townRoot, ".beads", "beads.db")); err == nil {
		workspaces = append(workspaces, townRoot)
	}

	// Rig workspaces
	rigs := discoverRigs(townRoot)
	for _, rigName := range rigs {
		rigPath := filepath.Join(townRoot, rigName)

		// Check for .beads directory with actual database
		beadsPath := filepath.Join(rigPath, ".beads")
		if _, err := os.Stat(beadsPath); err == nil {
			// Check if it's a redirect
			redirectPath := filepath.Join(beadsPath, "redirect")
			if _, err := os.Stat(redirectPath); err == nil {
				// It's a redirect - read target and resolve
				if content, err := os.ReadFile(redirectPath); err == nil {
					target := strings.TrimSpace(string(content))
					if !filepath.IsAbs(target) {
						target = filepath.Join(townRoot, target)
					}
					// Only add if the target actually exists and has a database
					if _, err := os.Stat(filepath.Join(target, "beads.db")); err == nil {
						workspaces = append(workspaces, target)
					}
				}
			} else if _, err := os.Stat(filepath.Join(beadsPath, "beads.db")); err == nil {
				// Not a redirect, has actual database
				workspaces = append(workspaces, rigPath)
			}
		}

		// Check mayor/rig subdirectory (common pattern)
		mayorRigPath := filepath.Join(rigPath, "mayor", "rig")
		if _, err := os.Stat(filepath.Join(mayorRigPath, ".beads", "beads.db")); err == nil {
			workspaces = append(workspaces, mayorRigPath)
		}
	}

	// Deduplicate and verify paths exist, use absolute paths
	seen := make(map[string]bool)
	unique := []string{}
	for _, ws := range workspaces {
		absPath, err := filepath.Abs(ws)
		if err != nil {
			continue
		}
		// Verify the path actually exists
		if _, err := os.Stat(absPath); err != nil {
			continue
		}
		if !seen[absPath] {
			seen[absPath] = true
			unique = append(unique, absPath) // Use absolute path
		}
	}

	return unique
}

// stopBdDaemon stops the bd daemon in a workspace.
func stopBdDaemon(workspace string) error {
	// bd daemons stop requires workspace path as argument
	cmd := exec.Command("bd", "daemons", "stop", workspace)
	return cmd.Run()
}

// startBdDaemon starts the bd daemon in a workspace.
func startBdDaemon(workspace string) error {
	cmd := exec.Command("bd", "daemon", "start")
	cmd.Dir = workspace
	output, err := cmd.CombinedOutput()
	if err != nil {
		// Check if it's already running (not an error)
		if strings.Contains(string(output), "already running") {
			return nil
		}
		return fmt.Errorf("%s: %s", err, string(output))
	}
	// Give daemon time to initialize
	time.Sleep(200 * time.Millisecond)
	return nil
}

// shortPath returns a shortened path for display.
func shortPath(path string) string {
	home, _ := os.UserHomeDir()
	if home != "" && strings.HasPrefix(path, home) {
		return "~" + path[len(home):]
	}
	return path
}
