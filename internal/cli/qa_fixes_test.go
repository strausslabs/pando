package cli

import (
	"encoding/json"
	"errors"
	"os"
	"strings"
	"testing"

	"github.com/guyStrauss/pando/internal/api"
)

func TestDaemonCmdHasAutoUpFlag(t *testing.T) {
	cmd := daemonCmd(&globalFlags{}, "v-test")
	if cmd.Flags().Lookup("auto-up") == nil {
		t.Error("daemon command must expose an --auto-up flag so the detached daemon can auto-up discovered worktrees")
	}
}

func TestPrintStatusShowsConfigError(t *testing.T) {
	st := []api.WorktreeStatus{{
		Worktree: "feat",
		Error:    "evaluate config: pando.star:3:1: undefined: srvice",
	}}
	var out, errOut string
	errOut = captureStderr(t, func() {
		out = captureStdout(t, func() { printStatus(st) })
	})
	if !strings.Contains(out, "config error") {
		t.Errorf("status table should flag the config error row:\n%s", out)
	}
	if !strings.Contains(errOut, "undefined: srvice") {
		t.Errorf("stderr should carry the config error detail:\n%s", errOut)
	}
}

func TestWorktreeIn(t *testing.T) {
	st := []api.WorktreeStatus{
		{Worktree: "a"},
		{Worktree: "b", Resources: []api.ResourceStatus{{Name: "x"}}},
	}
	got := worktreeIn(st, "b")
	if len(got) != 1 || got[0].Worktree != "b" {
		t.Fatalf("worktreeIn should isolate the named worktree, got %+v", got)
	}
	if worktreeIn(st, "missing") != nil {
		t.Error("worktreeIn should return nil for an unknown worktree")
	}
}

func TestUpPrintsConfirmationAndResources(t *testing.T) {
	ops := &stubOps{}
	g := &globalFlags{socket: liveDaemon(t, ops)}
	out := captureStdout(t, func() {
		if err := runCmd(upCmd(g), "--worktree", "main"); err != nil {
			t.Fatalf("up: %v", err)
		}
	})
	if !strings.Contains(out, "main is up") {
		t.Errorf("up should confirm the worktree is up:\n%s", out)
	}
	if !strings.Contains(out, "api") {
		t.Errorf("up should print the worktree's resources:\n%s", out)
	}
}

func TestUpJSONPrintsOnlyTargetWorktree(t *testing.T) {
	ops := &stubOps{}
	g := &globalFlags{socket: liveDaemon(t, ops), json: true}
	out := captureStdout(t, func() {
		if err := runCmd(upCmd(g), "--worktree", "main"); err != nil {
			t.Fatalf("up --json: %v", err)
		}
	})
	var got []api.WorktreeStatus
	if err := json.Unmarshal([]byte(out), &got); err != nil {
		t.Fatalf("up --json emitted invalid JSON: %v\n%s", err, out)
	}
	if len(got) != 1 || got[0].Worktree != "main" {
		t.Errorf("up --json should emit only the target worktree, got %+v", got)
	}
}

func TestNewClientWarnsOnCustomConfig(t *testing.T) {
	g := &globalFlags{socket: liveDaemon(t, &stubOps{}), config: "custom.star"}
	out := captureStderr(t, func() {
		if _, err := newClient(g); err != nil {
			t.Fatalf("newClient: %v", err)
		}
	})
	if !strings.Contains(out, "custom.star") || !strings.Contains(out, "ignored") {
		t.Errorf("newClient should warn that --config is ignored for a running daemon:\n%s", out)
	}
}

func TestNewClientNoWarnOnDefaultConfig(t *testing.T) {
	g := &globalFlags{socket: liveDaemon(t, &stubOps{}), config: defaultConfig}
	out := captureStderr(t, func() {
		if _, err := newClient(g); err != nil {
			t.Fatalf("newClient: %v", err)
		}
	})
	if strings.Contains(out, "ignored") {
		t.Errorf("default config should not warn:\n%s", out)
	}
}

func TestExecuteJSONErrorEmitsJSON(t *testing.T) {
	runtimeDir := t.TempDir()
	t.Setenv("XDG_RUNTIME_DIR", runtimeDir)
	t.Chdir(t.TempDir())

	var err error
	out := captureStdout(t, func() {
		savedArgs := os.Args
		os.Args = []string{"pando", "--json", "status"}
		defer func() { os.Args = savedArgs }()
		err = Execute("v-test")
	})

	if err == nil {
		t.Fatal("status with no daemon should error")
	}
	var handled Handled
	if !errors.As(err, &handled) {
		t.Errorf("a JSON-printed error should be wrapped in Handled, got %T", err)
	}
	var got map[string]string
	if jerr := json.Unmarshal([]byte(out), &got); jerr != nil {
		t.Fatalf("--json error should be valid JSON: %v\n%s", jerr, out)
	}
	if got["error"] == "" {
		t.Errorf("--json error JSON should carry an error field:\n%s", out)
	}
}
