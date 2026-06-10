package compose

import (
	"context"
	"os"
	"os/exec"
	"strings"
	"testing"
	"time"

	"github.com/guyStrauss/pando/internal/logbuf"
	"github.com/guyStrauss/pando/internal/resource"
	"github.com/guyStrauss/pando/internal/scheduler"
)

// dockerOrSkip skips when no usable Docker daemon is reachable, so the suite
// stays green in CI environments without Docker.
func dockerOrSkip(t *testing.T) {
	t.Helper()
	if exec.Command("docker", "info").Run() != nil {
		t.Skip("docker daemon not available; skipping integration test")
	}
}

type nopReporter struct{}

func (nopReporter) Phase(scheduler.Phase) {}
func (nopReporter) Logf(string, ...any)   {}

func testImage() string { return os.Getenv("PANDO_TEST_IMAGE") }

func intEnv() scheduler.Env {
	return scheduler.Env{Worktree: "itest", Project: "pando-itest", Ports: map[string]int{}, Vars: map[string]string{}}
}

func TestComposeLifecycleReal(t *testing.T) {
	dockerOrSkip(t)

	logs := logbuf.NewStore(1000)
	b, err := New(logs, time.Now)
	if err != nil {
		t.Fatal(err)
	}
	// Pull from GHCR rather than Docker Hub so CI never trips Docker Hub's
	// anonymous pull rate limit. Overridable for local runs.
	image := "ghcr.io/linuxcontainers/alpine:3.20"
	if env := testImage(); env != "" {
		image = env
	}
	r := &resource.Resource{
		Name: "ticker", Kind: resource.KindCompose,
		Compose: &resource.ComposeSpec{
			Image:   image,
			Command: []string{"sh", "-c", "echo CONTAINER-UP; i=0; while true; do echo tick-$i; i=$((i+1)); sleep 1; done"},
		},
	}
	env := intEnv()
	ctx := context.Background()

	// Best-effort pull so the test does not fail on a cold cache.
	_ = exec.Command("docker", "pull", image).Run()

	if err := b.Start(ctx, r, env, nopReporter{}); err != nil {
		t.Fatalf("start: %v", err)
	}
	defer b.Stop(ctx, r, env)

	if !waitForLog(logs, "itest", "ticker", "CONTAINER-UP", 15*time.Second) {
		t.Fatal("container startup log never captured")
	}

	res, err := b.Exec(ctx, "itest", "ticker", []string{"echo", "EXEC-OK"}, env)
	if err != nil {
		t.Fatalf("exec: %v", err)
	}
	if !strings.Contains(res.Stdout, "EXEC-OK") {
		t.Errorf("exec stdout: %q", res.Stdout)
	}
	if res.ExitCode != 0 {
		t.Errorf("exec exit code: %d", res.ExitCode)
	}

	// Non-zero exit code propagates.
	fail, err := b.Exec(ctx, "itest", "ticker", []string{"sh", "-c", "exit 7"}, env)
	if err != nil {
		t.Fatalf("exec fail: %v", err)
	}
	if fail.ExitCode != 7 {
		t.Errorf("expected exit 7, got %d", fail.ExitCode)
	}

	if err := b.Stop(ctx, r, env); err != nil {
		t.Errorf("stop: %v", err)
	}
}

func waitForLog(logs *logbuf.Store, wt, res, substr string, within time.Duration) bool {
	deadline := time.Now().Add(within)
	for time.Now().Before(deadline) {
		for _, line := range logs.Text(wt, res) {
			if strings.Contains(line, substr) {
				return true
			}
		}
		time.Sleep(100 * time.Millisecond)
	}
	return false
}
