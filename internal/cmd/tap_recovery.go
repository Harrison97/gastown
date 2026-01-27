package cmd

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"strings"
	"time"

	"github.com/spf13/cobra"
)

// HookInput represents the JSON input from Claude Code hooks
type HookInput struct {
	SessionID string `json:"session_id"`
	Cwd       string `json:"cwd"`
	ToolName  string `json:"tool_name"`
	ToolInput string `json:"tool_input"`
	Error     string `json:"error"`
}

var tapErrorRecoveryCmd = &cobra.Command{
	Use:   "error-recovery",
	Short: "Handle tool failures (PostToolUseFailure hook)",
	Long: `Handle tool execution failures for polecat recovery.

Called by Claude Code PostToolUseFailure hook when a tool fails.
Logs the error and notifies the witness for potential intervention.

This enables automatic detection of stuck polecats that hit API errors
or tool failures, allowing the witness to intervene if needed.`,
	RunE: runTapErrorRecovery,
}

var tapSessionEndCmd = &cobra.Command{
	Use:   "session-end",
	Short: "Handle session termination (SessionEnd hook)",
	Long: `Handle session termination cleanup.

Called by Claude Code SessionEnd hook when a session terminates.
Checks if work was left incomplete and notifies the witness.`,
	RunE: runTapSessionEnd,
}

func init() {
	tapCmd.AddCommand(tapErrorRecoveryCmd)
	tapCmd.AddCommand(tapSessionEndCmd)
}

func runTapErrorRecovery(cmd *cobra.Command, args []string) error {
	// Read hook input from stdin
	input, err := io.ReadAll(os.Stdin)
	if err != nil {
		// Can't read input, just log and continue
		logError("error-recovery: failed to read stdin: %v", err)
		return nil
	}

	var hookInput HookInput
	if err := json.Unmarshal(input, &hookInput); err != nil {
		// Not valid JSON, might be empty
		logError("error-recovery: failed to parse input: %v", err)
		return nil
	}

	// Log the error
	logError("TOOL FAILURE: tool=%s error=%s", hookInput.ToolName, hookInput.Error)

	// Check if this is an API error that needs recovery
	if isAPIError(hookInput.Error) {
		logError("API error detected, notifying witness for potential recovery")

		// Get polecat identity
		polecatName := os.Getenv("GT_POLECAT")
		rigName := os.Getenv("GT_RIG")

		if polecatName != "" && rigName != "" {
			// Notify witness via mail
			notifyWitness(rigName, polecatName, hookInput.Error)
		}
	}

	return nil
}

func runTapSessionEnd(cmd *cobra.Command, args []string) error {
	// Check if we're a polecat with work still hooked
	polecatName := os.Getenv("GT_POLECAT")
	rigName := os.Getenv("GT_RIG")

	if polecatName == "" {
		// Not a polecat, nothing to do
		return nil
	}

	logError("session-end: polecat=%s rig=%s", polecatName, rigName)

	// Check hook status - if work is still hooked, session ended unexpectedly
	// The witness will detect this via its normal patrol

	return nil
}

func isAPIError(errMsg string) bool {
	apiErrorIndicators := []string{
		"API Error",
		"400",
		"429",
		"500",
		"502",
		"503",
		"504",
		"tool use concurrency",
		"rate limit",
		"timeout",
	}

	errLower := strings.ToLower(errMsg)
	for _, indicator := range apiErrorIndicators {
		if strings.Contains(errLower, strings.ToLower(indicator)) {
			return true
		}
	}
	return false
}

func notifyWitness(rigName, polecatName, errorMsg string) {
	// Write to a file that the witness can check
	// This is simpler than sending mail for now
	errorFile := fmt.Sprintf("/home/harrison/gt/%s/polecats/%s/.error", rigName, polecatName)

	content := fmt.Sprintf("timestamp: %s\nerror: %s\n", time.Now().Format(time.RFC3339), errorMsg)

	if err := os.WriteFile(errorFile, []byte(content), 0644); err != nil {
		logError("failed to write error file: %v", err)
	}
}

func logError(format string, args ...interface{}) {
	// Log to stderr and to a log file
	msg := fmt.Sprintf(format, args...)
	fmt.Fprintln(os.Stderr, "[gt tap]", msg)

	// Also append to log file
	logFile := "/home/harrison/gt/.logs/tap-recovery.log"
	f, err := os.OpenFile(logFile, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err == nil {
		defer f.Close()
		fmt.Fprintf(f, "[%s] %s\n", time.Now().Format(time.RFC3339), msg)
	}
}
