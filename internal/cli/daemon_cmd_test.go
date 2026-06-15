package cli

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/strausslabs/pando/internal/client"
	"github.com/strausslabs/pando/internal/discovery"
)

func TestMCPCmdReturnsOnCancelledContext(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel() // already done: StdioTransport.Run returns immediately

	cmd := mcpCmd(&globalFlags{}, "v-test")
	cmd.SetContext(ctx)

	done := make(chan error, 1)
	go func() { done <- cmd.RunE(cmd, nil) }()
	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("mcp command did not return on cancelled context")
	}
}

func TestStopDaemonNoDaemon(t *testing.T) {
	runtimeDir, err := os.MkdirTemp("", "pd-rt")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(runtimeDir) })
	t.Setenv("XDG_RUNTIME_DIR", runtimeDir)
	t.Chdir(t.TempDir())

	cmd := stopCmd(&globalFlags{})
	cmd.SetArgs(nil)
	if err := cmd.Execute(); err == nil {
		t.Fatal("stop should error when no daemon is running")
	}
}

func TestWaitForSocketTimesOutWhenGoneNeverHappens(t *testing.T) {
	sock := liveDaemon(t, &stubOps{})
	if err := waitForSocket(context.Background(), sock, false, 300*time.Millisecond); err == nil {
		t.Fatal("waitForSocket(want=false) should time out while the daemon stays reachable")
	}
}

func TestDaemonCmdHasAutoUpFlag(t *testing.T) {
	cmd := daemonCmd(&globalFlags{}, "v-test")
	if cmd.Flags().Lookup("auto-up") == nil {
		t.Error("daemon command must expose an --auto-up flag so the detached daemon can auto-up discovered worktrees")
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

func TestExecuteHelp(t *testing.T) {
	savedOut, savedErr := os.Stdout, os.Stderr
	r, w, _ := os.Pipe()
	os.Stdout, os.Stderr = w, w
	saved := os.Args
	os.Args = []string{"pando", "--help"}
	err := Execute("v-test")
	os.Args = saved
	_ = w.Close()
	os.Stdout, os.Stderr = savedOut, savedErr
	if err != nil {
		t.Fatalf("Execute --help: %v", err)
	}
	out, _ := io.ReadAll(r)
	for _, want := range []string{"start", "stop", "daemon"} {
		if !strings.Contains(string(out), want) {
			t.Errorf("help output missing %q subcommand:\n%s", want, out)
		}
	}
}

func TestRunDaemonStartsAndShutsDown(t *testing.T) {
	if testing.Short() {
		t.Skip("brings up a real daemon")
	}
	runtimeDir, err := os.MkdirTemp("", "pd-rt")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(runtimeDir) })
	t.Setenv("XDG_RUNTIME_DIR", runtimeDir)

	repo := t.TempDir()
	gitCmd := func(args ...string) {
		c := exec.Command("git", args...)
		c.Dir = repo
		if out, err := c.CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v\n%s", args, err, out)
		}
	}
	gitCmd("init")
	gitCmd("config", "user.email", "t@t.dev")
	gitCmd("config", "user.name", "t")
	if err := os.WriteFile(filepath.Join(repo, "pando.star"), []byte(`
define_stack(
    name = "t",
    services = {"noop": service(task = task(cmd = "true"), runWhen = "once")},
)
`), 0o600); err != nil {
		t.Fatal(err)
	}
	gitCmd("add", "-A")
	gitCmd("commit", "-qm", "init")
	t.Chdir(repo)

	gitDir := discovery.GitCommonDir(context.Background())
	socket := discovery.SocketPath(gitDir)

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	g := &globalFlags{config: "pando.star"}
	go func() { done <- runDaemon(ctx, g, "v-test", "", false) }()

	deadline := time.Now().Add(10 * time.Second)
	healthy := false
	for time.Now().Before(deadline) {
		hctx, hcancel := context.WithTimeout(context.Background(), 200*time.Millisecond)
		err := client.New(socket).Health(hctx)
		hcancel()
		if err == nil {
			healthy = true
			break
		}
		time.Sleep(100 * time.Millisecond)
	}
	if !healthy {
		cancel()
		t.Fatal("daemon never became healthy")
	}

	if info, ok := discovery.Load(gitDir); !ok || info.Socket != socket {
		t.Errorf("discovery info not written correctly: %+v ok=%v", info, ok)
	}

	cancel()
	select {
	case err := <-done:
		if err != nil {
			t.Errorf("runDaemon returned %v, want nil on cancel", err)
		}
	case <-time.After(20 * time.Second):
		t.Fatal("runDaemon did not shut down after cancel")
	}
}
