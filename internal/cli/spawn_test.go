package cli

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/guyStrauss/pando/internal/client"
	"github.com/guyStrauss/pando/internal/discovery"
)

func TestUpAutoStartsDaemon(t *testing.T) {
	if testing.Short() {
		t.Skip("compiles pando and spawns a daemon")
	}
	repo := newTestRepo(t)
	gitDir := discovery.GitCommonDir(context.Background())

	if out, err := repo.run("up"); err != nil {
		t.Fatalf("first up: %v\n%s", err, out)
	}

	info, ok := discovery.Load(gitDir)
	if !ok {
		t.Fatal("daemon recorded no discovery info")
	}
	if want := fmt.Sprintf("127.0.0.1:%d", discovery.UIPort(gitDir)); info.UIAddr != want {
		t.Errorf("UI addr = %q, want repo-derived %q", info.UIAddr, want)
	}
	if err := reachable(info.Socket); err != nil {
		t.Fatalf("daemon unreachable after up: %v", err)
	}

	if out, err := repo.run("up"); err != nil {
		t.Fatalf("second up: %v\n%s", err, out)
	}
	if again, _ := discovery.Load(gitDir); again.PID != info.PID {
		t.Errorf("second up started a rival daemon (pid %d -> %d)", info.PID, again.PID)
	}

	if err := stopDaemon(context.Background()); err != nil {
		t.Fatalf("stop: %v", err)
	}
	if reachable(info.Socket) == nil {
		t.Error("daemon still reachable after stop")
	}
	if _, ok := discovery.Load(gitDir); ok {
		t.Error("discovery info not removed after stop")
	}
}

func TestUpInFreshWorktreeResolvesAndBringsUp(t *testing.T) {
	if testing.Short() {
		t.Skip("compiles pando and spawns a daemon")
	}
	repo := newTestRepo(t)

	if out, err := repo.run("up"); err != nil {
		t.Fatalf("up in main worktree: %v\n%s", err, out)
	}

	wtDir := filepath.Join(filepath.Dir(repo.dir), "wt-feat")
	t.Cleanup(func() { _ = os.RemoveAll(wtDir) })
	repo.git("worktree", "add", "-q", "-b", "feat", wtDir)

	feat := &testRepo{t: t, bin: repo.bin, dir: wtDir, env: repo.env}
	out, err := feat.run("up")
	if err != nil {
		t.Fatalf("first up in a freshly-added worktree must succeed, not race the reconciler: %v\n%s", err, out)
	}
	if !strings.Contains(out, "feat is up") {
		t.Errorf("up in the new worktree should confirm it came up:\n%s", out)
	}

	st, err := feat.run("status")
	if err != nil {
		t.Fatalf("status: %v\n%s", err, st)
	}
	if !strings.Contains(st, "feat") {
		t.Errorf("status should list the new worktree:\n%s", st)
	}
}

type testRepo struct {
	t   *testing.T
	bin string
	dir string
	env []string
}

func newTestRepo(t *testing.T) *testRepo {
	t.Helper()

	runtimeDir, err := os.MkdirTemp("", "pando-rt")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(runtimeDir) })
	t.Setenv("XDG_RUNTIME_DIR", runtimeDir)

	r := &testRepo{
		t:   t,
		bin: build(t),
		dir: t.TempDir(),
		env: append(os.Environ(), "XDG_RUNTIME_DIR="+runtimeDir),
	}
	r.git("init")
	r.git("config", "user.email", "t@t.dev")
	r.git("config", "user.name", "t")
	r.writeFile("pando.star", `
define_stack(
    name = "t",
    services = {"noop": service(task = task(cmd = "true"), runWhen = "once")},
)
`)
	r.git("add", "-A")
	r.git("commit", "-qm", "init")
	t.Chdir(r.dir)
	t.Cleanup(func() { _, _ = r.run("stop") })
	return r
}

func (r *testRepo) run(args ...string) (string, error) {
	r.t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, r.bin, args...)
	cmd.Dir = r.dir
	cmd.Env = r.env
	out, err := cmd.CombinedOutput()
	return string(out), err
}

func (r *testRepo) git(args ...string) {
	r.t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = r.dir
	if out, err := cmd.CombinedOutput(); err != nil {
		r.t.Fatalf("git %v: %v\n%s", args, err, out)
	}
}

func (r *testRepo) writeFile(name, content string) {
	r.t.Helper()
	if err := os.WriteFile(filepath.Join(r.dir, name), []byte(content), 0o600); err != nil {
		r.t.Fatal(err)
	}
}

func build(t *testing.T) string {
	t.Helper()
	bin := filepath.Join(t.TempDir(), "pando")
	if out, err := exec.Command("go", "build", "-o", bin, "github.com/guyStrauss/pando/cmd/pando").CombinedOutput(); err != nil {
		t.Fatalf("build pando: %v\n%s", err, out)
	}
	return bin
}

func reachable(socket string) error {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	return client.New(socket).Health(ctx)
}

func TestLastLogLine(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "daemon.log")

	if got := lastLogLine(filepath.Join(dir, "missing.log")); got != "" {
		t.Errorf("missing file should yield empty, got %q", got)
	}

	content := "pando ready\nerror: service \"x\": unknown field \"ignore\"\n\n\n"
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
	got := lastLogLine(path)
	if got != `error: service "x": unknown field "ignore"` {
		t.Errorf("lastLogLine should return the last non-blank line, got %q", got)
	}
}
