package cmd

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"
	"github.com/steveyegge/gastown/internal/beads"
	"github.com/steveyegge/gastown/internal/config"
	"github.com/steveyegge/gastown/internal/style"
	"github.com/steveyegge/gastown/internal/tmux"
	"github.com/steveyegge/gastown/internal/workspace"
)

var resetCmd = &cobra.Command{
	Use:   "reset",
	Short: "Reset Gas Town state to fresh install",
	Long: `Reset Gas Town to a clean state, as if freshly installed.

This command:
1. Stops running agents (deacon, polecats, etc. - mayor preserved by default)
2. Deletes the beads database (all issues, wisps, molecules)
3. Clears activity logs and event files
4. Recreates a fresh beads database
5. Preserves configuration (config.yaml, formulas, etc.)

Use --all to also stop the mayor.

Use this when you want a clean slate without reinstalling.

WARNING: This permanently deletes all work history. Use with caution.`,
	RunE: runReset,
}

var (
	resetForce bool
	resetAll   bool
)

func init() {
	resetCmd.Flags().BoolVarP(&resetForce, "force", "f", false, "Skip confirmation prompt")
	resetCmd.Flags().BoolVarP(&resetAll, "all", "a", false, "Also stop mayor (by default, mayor is preserved)")
	rootCmd.AddCommand(resetCmd)
}

func runReset(cmd *cobra.Command, args []string) error {
	townRoot, err := workspace.FindFromCwdOrError()
	if err != nil {
		return fmt.Errorf("not in a Gas Town workspace: %w", err)
	}

	// Confirmation prompt
	if !resetForce {
		fmt.Println("⚠️  This will permanently delete all Gas Town state:")
		fmt.Println("   - All issues, wisps, and molecules")
		fmt.Println("   - All activity history")
		fmt.Println("   - All hook and mail state")
		fmt.Println()
		fmt.Print("Type 'reset' to confirm: ")
		var confirm string
		fmt.Scanln(&confirm)
		if confirm != "reset" {
			fmt.Println("Aborted.")
			return nil
		}
	}

	fmt.Println()

	// Step 1: Stop all agents
	fmt.Println("Stopping agents...")
	t := tmux.NewTmux()

	// Stop deacon
	if running, _ := t.HasSession("hq-deacon"); running {
		_ = t.KillSessionWithProcesses("hq-deacon")
		fmt.Printf("  %s Stopped deacon\n", style.Bold.Render("✓"))
	}

	// Stop mayor (only with --all flag)
	if resetAll {
		if running, _ := t.HasSession("hq-mayor"); running {
			_ = t.KillSessionWithProcesses("hq-mayor")
			fmt.Printf("  %s Stopped mayor\n", style.Bold.Render("✓"))
		}
	}

	// Step 2: Stop beads daemon (it caches data in memory)
	fmt.Println("Stopping beads daemon...")
	stopDaemonCmd := exec.Command("bd", "daemon", "stop", townRoot)
	if err := stopDaemonCmd.Run(); err == nil {
		fmt.Printf("  %s Stopped beads daemon\n", style.Bold.Render("✓"))
	}

	// Step 3: Delete all beads with configured prefix (clears both local and global db)
	fmt.Println("Deleting all beads...")
	if err := deleteAllBeads(townRoot); err != nil {
		fmt.Printf("  %s Could not delete beads: %v\n", style.Dim.Render("Warning:"), err)
	}

	// Step 4: Delete beads databases (town + all agents)
	fmt.Println("Clearing databases...")
	beadsDirs := []string{
		filepath.Join(townRoot, ".beads"),
		filepath.Join(townRoot, "deacon", ".beads"),
		filepath.Join(townRoot, "mayor", ".beads"),
	}
	dbFiles := []string{
		"beads.db",
		"beads.db-shm",
		"beads.db-wal",
	}
	for _, beadsDir := range beadsDirs {
		for _, f := range dbFiles {
			path := filepath.Join(beadsDir, f)
			if err := os.Remove(path); err == nil {
				relPath := path[len(townRoot)+1:]
				fmt.Printf("  %s Deleted %s\n", style.Bold.Render("✓"), relPath)
			}
		}
	}

	// Step 5: Clear jsonl files and routes (these contain persisted beads data)
	fmt.Println("Clearing logs and beads data...")
	var jsonlFiles []string
	for _, beadsDir := range beadsDirs {
		jsonlFiles = append(jsonlFiles,
			filepath.Join(beadsDir, "issues.jsonl"),
			filepath.Join(beadsDir, "interactions.jsonl"),
			filepath.Join(beadsDir, "routes.jsonl"),
			filepath.Join(beadsDir, "molecules.jsonl"),
		)
	}
	jsonlFiles = append(jsonlFiles, filepath.Join(townRoot, ".events.jsonl"))
	for _, path := range jsonlFiles {
		if err := os.Remove(path); err == nil {
			relPath := path[len(townRoot)+1:]
			fmt.Printf("  %s Cleared %s\n", style.Bold.Render("✓"), relPath)
		}
	}

	// Step 6: Clear daemon activity
	activityPath := filepath.Join(townRoot, "daemon", "activity.json")
	if err := os.Remove(activityPath); err == nil {
		fmt.Printf("  %s Cleared daemon activity\n", style.Bold.Render("✓"))
	}

	// Step 7: Clear runtime state (session IDs, handoff state)
	fmt.Println("Clearing runtime state...")
	runtimeDirs := []string{
		filepath.Join(townRoot, ".runtime"),
		filepath.Join(townRoot, "mayor", ".runtime"),
		filepath.Join(townRoot, "deacon", ".runtime"),
	}
	for _, dir := range runtimeDirs {
		if entries, err := os.ReadDir(dir); err == nil {
			for _, entry := range entries {
				if !entry.IsDir() {
					path := filepath.Join(dir, entry.Name())
					if err := os.Remove(path); err == nil {
						fmt.Printf("  %s Cleared %s\n", style.Bold.Render("✓"), filepath.Join(filepath.Base(dir), entry.Name()))
					}
				}
			}
		}
	}

	// Step 8: Clear agent state files (deacon state, heartbeat)
	agentStateFiles := []string{
		filepath.Join(townRoot, "deacon", "state.json"),
		filepath.Join(townRoot, "deacon", "heartbeat.json"),
	}
	for _, path := range agentStateFiles {
		if err := os.Remove(path); err == nil {
			fmt.Printf("  %s Cleared %s\n", style.Bold.Render("✓"), filepath.Base(path))
		}
	}

	// Step 9: Recreate the beads database
	// Use --from-jsonl to prevent bd from scanning git history and reimporting old issues
	fmt.Println("Recreating beads database...")
	initCmd := exec.Command("bd", "init", "--quiet", "--from-jsonl")
	initCmd.Dir = townRoot
	if err := initCmd.Run(); err != nil {
		fmt.Printf("  %s Could not recreate database: %v\n", style.Dim.Render("Warning:"), err)
	} else {
		fmt.Printf("  %s Recreated beads database\n", style.Bold.Render("✓"))
	}

	// Step 10: Restore town prefix and routing configuration
	// Town beads use hq- prefix; this must be restored after db recreation
	fmt.Println("Restoring routing configuration...")

	// Restore hq- prefix for town beads
	prefixCmd := exec.Command("bd", "config", "set", "issue_prefix", "hq")
	prefixCmd.Dir = townRoot
	if err := prefixCmd.Run(); err != nil {
		fmt.Printf("  %s Could not restore town prefix: %v\n", style.Dim.Render("Warning:"), err)
	}

	// Restore allowed_prefixes for convoy beads (hq-cv-* IDs)
	allowedCmd := exec.Command("bd", "config", "set", "allowed_prefixes", "hq,hq-cv")
	allowedCmd.Dir = townRoot
	if err := allowedCmd.Run(); err != nil {
		fmt.Printf("  %s Could not restore allowed prefixes: %v\n", style.Dim.Render("Warning:"), err)
	}

	// Restore custom issue types for Gas Town
	typesCmd := exec.Command("bd", "config", "set", "types.custom", "agent,role,rig,convoy,slot,queue,event,message,molecule,gate,merge-request")
	typesCmd.Dir = townRoot
	if err := typesCmd.Run(); err != nil {
		fmt.Printf("  %s Could not restore custom types: %v\n", style.Dim.Render("Warning:"), err)
	}

	// Create empty issues.jsonl BEFORE routes.jsonl to prevent bd auto-export corruption.
	// If issues.jsonl doesn't exist, bd writes issue data to routes.jsonl (first .jsonl it finds).
	issuesPath := filepath.Join(townRoot, ".beads", "issues.jsonl")
	if _, err := os.Stat(issuesPath); os.IsNotExist(err) {
		if err := os.WriteFile(issuesPath, []byte{}, 0644); err != nil {
			fmt.Printf("  %s Could not create issues.jsonl: %v\n", style.Dim.Render("Warning:"), err)
		}
	}

	// Recreate routes.jsonl with town-level route
	if err := beads.AppendRoute(townRoot, beads.Route{Prefix: "hq-", Path: "."}); err != nil {
		fmt.Printf("  %s Could not restore town route: %v\n", style.Dim.Render("Warning:"), err)
	}

	// Restore rig routes from rigs.json
	rigsPath := filepath.Join(townRoot, "mayor", "rigs.json")
	if rigsConfig, err := config.LoadRigsConfig(rigsPath); err == nil {
		for rigName, rig := range rigsConfig.Rigs {
			if rig.BeadsConfig != nil && rig.BeadsConfig.Prefix != "" {
				// Route to mayor/rig if it exists, otherwise to rig root
				routePath := rigName
				mayorRigBeads := filepath.Join(townRoot, rigName, "mayor", "rig", ".beads")
				if _, err := os.Stat(mayorRigBeads); err == nil {
					routePath = rigName + "/mayor/rig"
				}
				route := beads.Route{Prefix: rig.BeadsConfig.Prefix + "-", Path: routePath}
				if err := beads.AppendRoute(townRoot, route); err != nil {
					fmt.Printf("  %s Could not restore route for %s: %v\n", style.Dim.Render("Warning:"), rigName, err)
				}
			}
		}
	}
	fmt.Printf("  %s Restored routing configuration\n", style.Bold.Render("✓"))

	fmt.Println()
	fmt.Printf("%s Gas Town reset to clean state\n", style.Bold.Render("✓"))
	fmt.Println("  Configuration preserved (config.yaml, formulas)")
	fmt.Println("  Run 'gt status' to verify")

	return nil
}

// deleteAllBeads deletes all beads with the configured issue prefix.
// This ensures both local and global beads databases are cleaned up.
func deleteAllBeads(townRoot string) error {
	// Get the issue prefix
	prefixCmd := exec.Command("bd", "config", "get", "issue_prefix")
	prefixCmd.Dir = townRoot
	prefixOut, err := prefixCmd.Output()
	if err != nil {
		return fmt.Errorf("getting issue prefix: %w", err)
	}
	prefix := strings.TrimSpace(string(prefixOut))
	if prefix == "" {
		return fmt.Errorf("no issue prefix configured")
	}

	// List all issues with this prefix (using --no-daemon to get direct access)
	listCmd := exec.Command("bd", "--no-daemon", "list", "--json")
	listCmd.Dir = townRoot
	listOut, err := listCmd.Output()
	if err != nil {
		// No issues to delete
		return nil
	}

	// Parse the JSON output to get issue IDs
	var issues []struct {
		ID string `json:"id"`
	}
	if err := json.Unmarshal(listOut, &issues); err != nil {
		return fmt.Errorf("parsing issue list: %w", err)
	}

	// Filter to issues with our prefix
	var idsToDelete []string
	for _, issue := range issues {
		if strings.HasPrefix(issue.ID, prefix+"-") {
			idsToDelete = append(idsToDelete, issue.ID)
		}
	}

	if len(idsToDelete) == 0 {
		fmt.Printf("  %s No beads to delete\n", style.Dim.Render("·"))
		return nil
	}

	// Delete all issues with cascade (to get children) and hard (permanent)
	deleteArgs := []string{"--no-daemon", "delete", "--cascade", "--hard", "--force"}
	deleteArgs = append(deleteArgs, idsToDelete...)
	deleteCmd := exec.Command("bd", deleteArgs...)
	deleteCmd.Dir = townRoot
	if err := deleteCmd.Run(); err != nil {
		return fmt.Errorf("deleting beads: %w", err)
	}

	fmt.Printf("  %s Deleted %d beads\n", style.Bold.Render("✓"), len(idsToDelete))
	return nil
}
