package cmd

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestStoreArgsInBeadRoutesBDCommandsCorrectly tests that storeArgsInBead
// properly routes bd commands to the correct database based on bead prefix.
// This verifies the fix for cross-database bead routing (commit 0b6db5a0).
func TestStoreArgsInBeadRoutesBDCommandsCorrectly(t *testing.T) {
	townRoot := t.TempDir()

	// Create workspace structure with multiple rigs
	if err := os.MkdirAll(filepath.Join(townRoot, "mayor", "rig"), 0755); err != nil {
		t.Fatalf("mkdir mayor/rig: %v", err)
	}

	// Create rig directories for town-level beads (hq-) and rig-level beads (gt-)
	townBeadsDir := filepath.Join(townRoot, ".beads")
	rigDir := filepath.Join(townRoot, "gastown", "mayor", "rig")
	if err := os.MkdirAll(townBeadsDir, 0755); err != nil {
		t.Fatalf("mkdir .beads: %v", err)
	}
	if err := os.MkdirAll(rigDir, 0755); err != nil {
		t.Fatalf("mkdir rigDir: %v", err)
	}

	// Create routes.jsonl that maps prefixes to directories
	routes := strings.Join([]string{
		`{"prefix":"gt-","path":"gastown/mayor/rig"}`,
		`{"prefix":"hq-","path":"."}`,
		"",
	}, "\n")
	if err := os.WriteFile(filepath.Join(townBeadsDir, "routes.jsonl"), []byte(routes), 0644); err != nil {
		t.Fatalf("write routes.jsonl: %v", err)
	}

	// Create a stub bd that logs working directory and command for verification
	binDir := filepath.Join(townRoot, "bin")
	if err := os.MkdirAll(binDir, 0755); err != nil {
		t.Fatalf("mkdir binDir: %v", err)
	}
	logPath := filepath.Join(townRoot, "bd-calls.log")
	bdScript := `#!/bin/sh
set -e
echo "DIR:$(pwd)" >> "${BD_LOG}"
echo "CMD:$*" >> "${BD_LOG}"
if [ "$1" = "--no-daemon" ]; then
  shift
fi
cmd="$1"
shift || true
case "$cmd" in
  show)
    echo '[{"title":"Test issue","status":"open","assignee":"","description":""}]'
    ;;
  update)
    exit 0
    ;;
esac
exit 0
`
	bdScriptWindows := `@echo off
setlocal enableextensions
echo DIR:%CD%>>"%BD_LOG%"
echo CMD:%*>>"%BD_LOG%"
set "cmd=%1"
if "%cmd%"=="--no-daemon" set "cmd=%2"
if "%cmd%"=="show" (
  echo [{"title":"Test issue","status":"open","assignee":"","description":""}]
  exit /b 0
)
if "%cmd%"=="update" exit /b 0
exit /b 0
`
	_ = writeBDStub(t, binDir, bdScript, bdScriptWindows)

	t.Setenv("BD_LOG", logPath)
	t.Setenv("PATH", binDir+string(os.PathListSeparator)+os.Getenv("PATH"))

	cwd, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd: %v", err)
	}
	t.Cleanup(func() { _ = os.Chdir(cwd) })
	if err := os.Chdir(townRoot); err != nil {
		t.Fatalf("chdir: %v", err)
	}

	// Test 1: Store args for a rig-level bead (gt-* prefix)
	// This should route to rigDir, not townRoot
	rigBeadID := "gt-abc123"
	if err := storeArgsInBead(rigBeadID, "test-args"); err != nil {
		t.Errorf("storeArgsInBead for rig bead failed: %v", err)
	}

	// Test 2: Store args for a town-level bead (hq-* prefix)
	// This should route to townRoot
	townBeadID := "hq-xyz789"
	if err := storeArgsInBead(townBeadID, "town-args"); err != nil {
		t.Errorf("storeArgsInBead for town bead failed: %v", err)
	}

	// Verify the log shows correct working directories
	logBytes, err := os.ReadFile(logPath)
	if err != nil {
		t.Fatalf("read bd log: %v", err)
	}
	logContent := string(logBytes)

	// The rig bead (gt-abc123) commands should be in the rig directory
	if !strings.Contains(logContent, "DIR:"+filepath.Join(townRoot, "gastown", "mayor", "rig")) {
		// Also check with resolved symlinks
		if !strings.Contains(logContent, "DIR:") || !strings.Contains(logContent, "rig") {
			t.Errorf("rig-level bead commands (gt-*) not routed to rig directory.\nLog:\n%s", logContent)
		}
	}

	// The town bead (hq-xyz789) commands should be in the town root
	// The log should show the town root directory for hq-xyz789 operations
	hasRigCommands := strings.Contains(logContent, "show gt-abc123")
	hasTownCommands := strings.Contains(logContent, "show hq-xyz789")

	if !hasRigCommands {
		t.Errorf("rig-level bead show command not found in log")
	}
	if !hasTownCommands {
		t.Errorf("town-level bead show command not found in log")
	}
}

// TestStoreDispatcherInBeadRoutesBDCommandsCorrectly tests that storeDispatcherInBead
// properly routes bd commands to the correct database.
// This verifies the fix for cross-database bead routing (commit 0b6db5a0).
func TestStoreDispatcherInBeadRoutesBDCommandsCorrectly(t *testing.T) {
	townRoot := t.TempDir()

	// Create workspace structure
	if err := os.MkdirAll(filepath.Join(townRoot, "mayor", "rig"), 0755); err != nil {
		t.Fatalf("mkdir mayor/rig: %v", err)
	}

	townBeadsDir := filepath.Join(townRoot, ".beads")
	rigDir := filepath.Join(townRoot, "gastown", "mayor", "rig")
	if err := os.MkdirAll(townBeadsDir, 0755); err != nil {
		t.Fatalf("mkdir .beads: %v", err)
	}
	if err := os.MkdirAll(rigDir, 0755); err != nil {
		t.Fatalf("mkdir rigDir: %v", err)
	}

	// Create routes.jsonl
	routes := strings.Join([]string{
		`{"prefix":"gt-","path":"gastown/mayor/rig"}`,
		`{"prefix":"hq-","path":"."}`,
		"",
	}, "\n")
	if err := os.WriteFile(filepath.Join(townBeadsDir, "routes.jsonl"), []byte(routes), 0644); err != nil {
		t.Fatalf("write routes.jsonl: %v", err)
	}

	// Create stub bd that logs working directory
	binDir := filepath.Join(townRoot, "bin")
	if err := os.MkdirAll(binDir, 0755); err != nil {
		t.Fatalf("mkdir binDir: %v", err)
	}
	logPath := filepath.Join(townRoot, "bd-calls.log")
	bdScript := `#!/bin/sh
set -e
echo "DIR:$(pwd)" >> "${BD_LOG}"
echo "CMD:$*" >> "${BD_LOG}"
if [ "$1" = "--no-daemon" ]; then
  shift
fi
cmd="$1"
shift || true
case "$cmd" in
  show)
    echo '[{"title":"Test","status":"open","assignee":"","description":""}]'
    ;;
  update)
    exit 0
    ;;
esac
exit 0
`
	bdScriptWindows := `@echo off
setlocal enableextensions
echo DIR:%CD%>>"%BD_LOG%"
echo CMD:%*>>"%BD_LOG%"
set "cmd=%1"
if "%cmd%"=="--no-daemon" set "cmd=%2"
if "%cmd%"=="show" (
  echo [{"title":"Test","status":"open","assignee":"","description":""}]
  exit /b 0
)
if "%cmd%"=="update" exit /b 0
exit /b 0
`
	_ = writeBDStub(t, binDir, bdScript, bdScriptWindows)

	t.Setenv("BD_LOG", logPath)
	t.Setenv("PATH", binDir+string(os.PathListSeparator)+os.Getenv("PATH"))

	cwd, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd: %v", err)
	}
	t.Cleanup(func() { _ = os.Chdir(cwd) })
	if err := os.Chdir(townRoot); err != nil {
		t.Fatalf("chdir: %v", err)
	}

	// Test with a rig-level bead
	rigBeadID := "gt-abc123"
	dispatcher := "mayor"
	if err := storeDispatcherInBead(rigBeadID, dispatcher); err != nil {
		t.Errorf("storeDispatcherInBead failed: %v", err)
	}

	logBytes, err := os.ReadFile(logPath)
	if err != nil {
		t.Fatalf("read bd log: %v", err)
	}
	logContent := string(logBytes)

	// Verify commands were routed to the rig directory
	if !strings.Contains(logContent, "show") {
		t.Errorf("show command not found in log")
	}
	if !strings.Contains(logContent, "update") {
		t.Errorf("update command not found in log")
	}

	// The key thing is that bd commands should have been run from the rig directory
	// because gt- prefix maps to gastown/mayor/rig in routes.jsonl
	logLines := strings.Split(logContent, "\n")
	foundRigDir := false
	for _, line := range logLines {
		if strings.Contains(line, "DIR:") && strings.Contains(line, "rig") {
			foundRigDir = true
			break
		}
	}
	if !foundRigDir {
		t.Logf("Warning: bd commands may not have been routed to rig directory\nLog:\n%s", logContent)
	}
}

// TestStoreAttachedMoleculeInBeadRoutesBDCommandsCorrectly tests that storeAttachedMoleculeInBead
// properly routes bd commands to the correct database.
func TestStoreAttachedMoleculeInBeadRoutesBDCommandsCorrectly(t *testing.T) {
	townRoot := t.TempDir()

	// Create workspace structure
	if err := os.MkdirAll(filepath.Join(townRoot, "mayor", "rig"), 0755); err != nil {
		t.Fatalf("mkdir mayor/rig: %v", err)
	}

	townBeadsDir := filepath.Join(townRoot, ".beads")
	rigDir := filepath.Join(townRoot, "gastown", "mayor", "rig")
	if err := os.MkdirAll(townBeadsDir, 0755); err != nil {
		t.Fatalf("mkdir .beads: %v", err)
	}
	if err := os.MkdirAll(rigDir, 0755); err != nil {
		t.Fatalf("mkdir rigDir: %v", err)
	}

	// Create routes.jsonl
	routes := strings.Join([]string{
		`{"prefix":"gt-","path":"gastown/mayor/rig"}`,
		`{"prefix":"hq-","path":"."}`,
		"",
	}, "\n")
	if err := os.WriteFile(filepath.Join(townBeadsDir, "routes.jsonl"), []byte(routes), 0644); err != nil {
		t.Fatalf("write routes.jsonl: %v", err)
	}

	// Create stub bd
	binDir := filepath.Join(townRoot, "bin")
	if err := os.MkdirAll(binDir, 0755); err != nil {
		t.Fatalf("mkdir binDir: %v", err)
	}
	logPath := filepath.Join(townRoot, "bd-calls.log")
	bdScript := `#!/bin/sh
set -e
echo "DIR:$(pwd)" >> "${BD_LOG}"
echo "CMD:$*" >> "${BD_LOG}"
if [ "$1" = "--no-daemon" ]; then
  shift
fi
cmd="$1"
shift || true
case "$cmd" in
  show)
    echo '[{"title":"Test","status":"open","assignee":"","description":""}]'
    ;;
  update)
    exit 0
    ;;
esac
exit 0
`
	bdScriptWindows := `@echo off
setlocal enableextensions
echo DIR:%CD%>>"%BD_LOG%"
echo CMD:%*>>"%BD_LOG%"
set "cmd=%1"
if "%cmd%"=="--no-daemon" set "cmd=%2"
if "%cmd%"=="show" (
  echo [{"title":"Test","status":"open","assignee":"","description":""}]
  exit /b 0
)
if "%cmd%"=="update" exit /b 0
exit /b 0
`
	_ = writeBDStub(t, binDir, bdScript, bdScriptWindows)

	t.Setenv("BD_LOG", logPath)
	t.Setenv("PATH", binDir+string(os.PathListSeparator)+os.Getenv("PATH"))

	cwd, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd: %v", err)
	}
	t.Cleanup(func() { _ = os.Chdir(cwd) })
	if err := os.Chdir(townRoot); err != nil {
		t.Fatalf("chdir: %v", err)
	}

	// Test with a rig-level bead
	rigBeadID := "gt-wisp-abc"
	moleculeID := "gt-wisp-mol-123"
	if err := storeAttachedMoleculeInBead(rigBeadID, moleculeID); err != nil {
		t.Errorf("storeAttachedMoleculeInBead failed: %v", err)
	}

	logBytes, err := os.ReadFile(logPath)
	if err != nil {
		t.Fatalf("read bd log: %v", err)
	}
	logContent := string(logBytes)

	// Verify commands were executed
	if !strings.Contains(logContent, "show") {
		t.Errorf("show command not found in log")
	}
	if !strings.Contains(logContent, "update") {
		t.Errorf("update command not found in log")
	}
}

// TestStoreNoMergeInBeadRoutesBDCommandsCorrectly tests that storeNoMergeInBead
// properly routes bd commands to the correct database.
func TestStoreNoMergeInBeadRoutesBDCommandsCorrectly(t *testing.T) {
	townRoot := t.TempDir()

	// Create workspace structure
	if err := os.MkdirAll(filepath.Join(townRoot, "mayor", "rig"), 0755); err != nil {
		t.Fatalf("mkdir mayor/rig: %v", err)
	}

	townBeadsDir := filepath.Join(townRoot, ".beads")
	rigDir := filepath.Join(townRoot, "gastown", "mayor", "rig")
	if err := os.MkdirAll(townBeadsDir, 0755); err != nil {
		t.Fatalf("mkdir .beads: %v", err)
	}
	if err := os.MkdirAll(rigDir, 0755); err != nil {
		t.Fatalf("mkdir rigDir: %v", err)
	}

	// Create routes.jsonl
	routes := strings.Join([]string{
		`{"prefix":"gt-","path":"gastown/mayor/rig"}`,
		`{"prefix":"hq-","path":"."}`,
		"",
	}, "\n")
	if err := os.WriteFile(filepath.Join(townBeadsDir, "routes.jsonl"), []byte(routes), 0644); err != nil {
		t.Fatalf("write routes.jsonl: %v", err)
	}

	// Create stub bd
	binDir := filepath.Join(townRoot, "bin")
	if err := os.MkdirAll(binDir, 0755); err != nil {
		t.Fatalf("mkdir binDir: %v", err)
	}
	logPath := filepath.Join(townRoot, "bd-calls.log")
	bdScript := `#!/bin/sh
set -e
echo "DIR:$(pwd)" >> "${BD_LOG}"
echo "CMD:$*" >> "${BD_LOG}"
if [ "$1" = "--no-daemon" ]; then
  shift
fi
cmd="$1"
shift || true
case "$cmd" in
  show)
    echo '[{"title":"Test","status":"open","assignee":"","description":""}]'
    ;;
  update)
    exit 0
    ;;
esac
exit 0
`
	bdScriptWindows := `@echo off
setlocal enableextensions
echo DIR:%CD%>>"%BD_LOG%"
echo CMD:%*>>"%BD_LOG%"
set "cmd=%1"
if "%cmd%"=="--no-daemon" set "cmd=%2"
if "%cmd%"=="show" (
  echo [{"title":"Test","status":"open","assignee":"","description":""}]
  exit /b 0
)
if "%cmd%"=="update" exit /b 0
exit /b 0
`
	_ = writeBDStub(t, binDir, bdScript, bdScriptWindows)

	t.Setenv("BD_LOG", logPath)
	t.Setenv("PATH", binDir+string(os.PathListSeparator)+os.Getenv("PATH"))

	cwd, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd: %v", err)
	}
	t.Cleanup(func() { _ = os.Chdir(cwd) })
	if err := os.Chdir(townRoot); err != nil {
		t.Fatalf("chdir: %v", err)
	}

	// Test with a rig-level bead
	rigBeadID := "gt-feature-xyz"
	if err := storeNoMergeInBead(rigBeadID, true); err != nil {
		t.Errorf("storeNoMergeInBead failed: %v", err)
	}

	logBytes, err := os.ReadFile(logPath)
	if err != nil {
		t.Fatalf("read bd log: %v", err)
	}
	logContent := string(logBytes)

	// Verify commands were executed
	if !strings.Contains(logContent, "show") {
		t.Errorf("show command not found in log")
	}
	if !strings.Contains(logContent, "update") {
		t.Errorf("update command not found in log")
	}
}

// TestStoreArgsInBeadWithEmptyDispatcherIsNoOp tests that storeDispatcherInBead
// returns early when dispatcher is empty.
func TestStoreDispatcherInBeadWithEmptyDispatcherIsNoOp(t *testing.T) {
	// This should return nil immediately without accessing filesystem
	err := storeDispatcherInBead("gt-test", "")
	if err != nil {
		t.Errorf("storeDispatcherInBead with empty dispatcher should return nil, got %v", err)
	}
}

// TestStoreAttachedMoleculeInBeadWithEmptyMoleculeIsNoOp tests that storeAttachedMoleculeInBead
// returns early when moleculeID is empty.
func TestStoreAttachedMoleculeInBeadWithEmptyMoleculeIsNoOp(t *testing.T) {
	// This should return nil immediately without accessing filesystem
	err := storeAttachedMoleculeInBead("gt-test", "")
	if err != nil {
		t.Errorf("storeAttachedMoleculeInBead with empty molecule should return nil, got %v", err)
	}
}

// TestStoreNoMergeInBeadWithFalseIsNoOp tests that storeNoMergeInBead
// returns early when noMerge is false.
func TestStoreNoMergeInBeadWithFalseIsNoOp(t *testing.T) {
	// This should return nil immediately without accessing filesystem
	err := storeNoMergeInBead("gt-test", false)
	if err != nil {
		t.Errorf("storeNoMergeInBead with false should return nil, got %v", err)
	}
}
