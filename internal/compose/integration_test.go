package compose

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/guyStrauss/pando/internal/logbuf"
	"github.com/guyStrauss/pando/internal/resource"
	"github.com/guyStrauss/pando/internal/scheduler"
)

func ensureDocker(t *testing.T) {
	t.Helper()
	if exec.Command("docker", "info").Run() != nil {
		t.Fatal("docker daemon not available; this integration test requires Docker")
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
	ensureDocker(t)

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
	defer func() { _ = b.Stop(ctx, r, env) }()

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

	if usage, ok := b.Sample(ctx, r, env); !ok {
		t.Error("Sample should report stats for a running container")
	} else if usage.MemBytes == 0 {
		t.Error("Sample returned zero memory for a running container")
	}

	if err := b.Stop(ctx, r, env); err != nil {
		t.Errorf("stop: %v", err)
	}
}

func TestComposeBuildAndSyncReal(t *testing.T) {
	ensureDocker(t)

	dir := t.TempDir()
	mustWrite(t, filepath.Join(dir, "Dockerfile"),
		"FROM ghcr.io/linuxcontainers/alpine:3.20\nRUN echo built > /built.txt\nCMD [\"sleep\",\"3600\"]\n")

	logs := logbuf.NewStore(1000)
	b, err := New(logs, time.Now)
	if err != nil {
		t.Fatal(err)
	}
	env := scheduler.Env{Worktree: "itest", Project: "pando-build-itest", Ports: map[string]int{}, Vars: map[string]string{}}
	r := &resource.Resource{
		Name: "built", Kind: resource.KindCompose,
		Build: &resource.Build{Context: dir},
	}
	ctx := context.Background()

	if err := b.Start(ctx, r, env, nopReporter{}); err != nil {
		t.Fatalf("start (build): %v", err)
	}
	defer func() { _ = b.Stop(ctx, r, env) }()

	res, err := b.Exec(ctx, "itest", "built", []string{"cat", "/built.txt"}, env)
	if err != nil {
		t.Fatalf("exec after build: %v", err)
	}
	if !strings.Contains(res.Stdout, "built") {
		t.Errorf("built image missing RUN artifact: %q", res.Stdout)
	}

	host := t.TempDir()
	mustWrite(t, filepath.Join(host, "synced.txt"), "from-host")
	if err := b.Sync(ctx, r, env, filepath.Join(host, "synced.txt"), "/tmp/synced.txt"); err != nil {
		t.Fatalf("sync: %v", err)
	}
	got, err := b.Exec(ctx, "itest", "built", []string{"cat", "/tmp/synced.txt"}, env)
	if err != nil {
		t.Fatalf("exec after sync: %v", err)
	}
	if !strings.Contains(got.Stdout, "from-host") {
		t.Errorf("synced file content wrong: %q", got.Stdout)
	}
}

func TestRestartContainerPreservesSyncedFileReal(t *testing.T) {
	ensureDocker(t)

	logs := logbuf.NewStore(1000)
	b, err := New(logs, time.Now)
	if err != nil {
		t.Fatal(err)
	}
	env := scheduler.Env{Worktree: "itest", Project: "pando-restart-itest", Ports: map[string]int{}, Vars: map[string]string{}}
	r := &resource.Resource{
		Name: "app", Kind: resource.KindCompose,
		Compose: &resource.ComposeSpec{Image: "ghcr.io/linuxcontainers/alpine:3.20", Command: []string{"sleep", "3600"}},
	}
	ctx := context.Background()

	if err := b.Start(ctx, r, env, nopReporter{}); err != nil {
		t.Fatalf("start: %v", err)
	}
	defer func() { _ = b.Stop(ctx, r, env) }()

	host := t.TempDir()
	mustWrite(t, filepath.Join(host, "synced.txt"), "live-edit")
	if err := b.Sync(ctx, r, env, filepath.Join(host, "synced.txt"), "/tmp/synced.txt"); err != nil {
		t.Fatalf("sync: %v", err)
	}

	if err := b.RestartContainer(ctx, r, env); err != nil {
		t.Fatalf("restart_container: %v", err)
	}

	got, err := b.Exec(ctx, "itest", "app", []string{"cat", "/tmp/synced.txt"}, env)
	if err != nil {
		t.Fatalf("exec after restart: %v", err)
	}
	if !strings.Contains(got.Stdout, "live-edit") {
		t.Errorf("restart_container dropped the synced file (should survive an in-place restart): %q", got.Stdout)
	}
}

func TestSyncCreatesParentDirReal(t *testing.T) {
	ensureDocker(t)

	logs := logbuf.NewStore(1000)
	b, err := New(logs, time.Now)
	if err != nil {
		t.Fatal(err)
	}
	env := scheduler.Env{Worktree: "itest", Project: "pando-syncdir-itest", Ports: map[string]int{}, Vars: map[string]string{}}
	r := &resource.Resource{
		Name: "app", Kind: resource.KindCompose,
		Compose: &resource.ComposeSpec{Image: "ghcr.io/linuxcontainers/alpine:3.20", Command: []string{"sleep", "3600"}},
	}
	ctx := context.Background()

	if err := b.Start(ctx, r, env, nopReporter{}); err != nil {
		t.Fatalf("start: %v", err)
	}
	defer func() { _ = b.Stop(ctx, r, env) }()

	host := t.TempDir()
	mustWrite(t, filepath.Join(host, "app.jar"), "artifact")
	// /app does not exist in a bare alpine image; sync must create it.
	if err := b.Sync(ctx, r, env, filepath.Join(host, "app.jar"), "/app/app.jar"); err != nil {
		t.Fatalf("sync into a missing dir should create it, got: %v", err)
	}
	got, err := b.Exec(ctx, "itest", "app", []string{"cat", "/app/app.jar"}, env)
	if err != nil {
		t.Fatalf("exec: %v", err)
	}
	if !strings.Contains(got.Stdout, "artifact") {
		t.Errorf("synced artifact not found at /app/app.jar: %q", got.Stdout)
	}
}

func mustWrite(t *testing.T, path, content string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatal(err)
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
