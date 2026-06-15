package cli

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"

	"github.com/guyStrauss/pando/internal/client"
	"github.com/guyStrauss/pando/internal/discovery"
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
