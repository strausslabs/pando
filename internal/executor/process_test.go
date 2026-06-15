package executor

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/guyStrauss/pando/internal/logbuf"
	"github.com/guyStrauss/pando/internal/resource"
	"github.com/guyStrauss/pando/internal/scheduler"
)

type nopReporter struct{ phases []scheduler.Phase }

func (n *nopReporter) Phase(p scheduler.Phase) { n.phases = append(n.phases, p) }
func (n *nopReporter) Logf(string, ...any)     {}

func fixedClock() time.Time { return time.Unix(1700000000, 0) }

func newTestEngine() (*Engine, *logbuf.Store) {
	store := logbuf.NewStore(1000)
	return NewEngine(store, fixedClock), store
}

func taskRes(name, cmd string) *resource.Resource {
	return &resource.Resource{Name: name, Kind: resource.KindTask, Task: &resource.TaskSpec{Cmd: cmd}}
}

func localRes(name, cmd string) *resource.Resource {
	return &resource.Resource{Name: name, Kind: resource.KindLocal, Local: &resource.LocalSpec{Cmd: cmd}}
}

func collectText(store *logbuf.Store, wt, name string) string {
	lines, _ := store.Query(wt, name, logbuf.Query{})
	var b strings.Builder
	for _, l := range lines {
		b.WriteString(l.Text)
		b.WriteString("\n")
	}
	return b.String()
}

func TestTaskCapturesStdout(t *testing.T) {
	e, store := newTestEngine()
	r := taskRes("hello", "echo hello world")
	err := e.Start(context.Background(), r, scheduler.Env{Worktree: "main"}, &nopReporter{})
	if err != nil {
		t.Fatalf("task: %v", err)
	}
	if !strings.Contains(collectText(store, "main", "hello"), "hello world") {
		t.Errorf("stdout not captured: %q", collectText(store, "main", "hello"))
	}
}

func TestTaskCapturesStderr(t *testing.T) {
	e, store := newTestEngine()
	r := taskRes("err", "echo oops 1>&2")
	_ = e.Start(context.Background(), r, scheduler.Env{Worktree: "main"}, &nopReporter{})
	lines, _ := store.Query("main", "err", logbuf.Query{})
	found := false
	for _, l := range lines {
		if l.Stream == logbuf.Stderr && strings.Contains(l.Text, "oops") {
			found = true
		}
	}
	if !found {
		t.Error("stderr not captured with stderr stream")
	}
}

func TestTaskNonZeroExitFails(t *testing.T) {
	e, _ := newTestEngine()
	r := taskRes("fail", "exit 3")
	err := e.Start(context.Background(), r, scheduler.Env{Worktree: "main"}, &nopReporter{})
	if err == nil {
		t.Fatal("expected error from non-zero exit")
	}
}

func TestTaskInterpolatesPortAndEnv(t *testing.T) {
	e, store := newTestEngine()
	r := taskRes("interp", "echo port=$PORT_api env=$MSG")
	r.Task.Env = map[string]string{"MSG": "hi-$PORT_api"}
	env := scheduler.Env{Worktree: "main", Ports: map[string]int{"api": 8042}}
	if err := e.Start(context.Background(), r, env, &nopReporter{}); err != nil {
		t.Fatal(err)
	}
	out := collectText(store, "main", "interp")
	if !strings.Contains(out, "port=8042") {
		t.Errorf("port not interpolated in cmd: %q", out)
	}
	if !strings.Contains(out, "env=hi-8042") {
		t.Errorf("port not interpolated in env: %q", out)
	}
}

func TestTaskBadInterpolationErrors(t *testing.T) {
	e, _ := newTestEngine()
	r := taskRes("bad", "echo $PORT_missing")
	err := e.Start(context.Background(), r, scheduler.Env{Worktree: "main"}, &nopReporter{})
	if err == nil {
		t.Fatal("undefined port should error before exec")
	}
}

func TestLocalRunsAndStops(t *testing.T) {
	e, store := newTestEngine()
	r := localRes("loop", "while true; do echo tick; sleep 0.05; done")
	rep := &nopReporter{}
	if err := e.Start(context.Background(), r, scheduler.Env{Worktree: "main"}, rep); err != nil {
		t.Fatal(err)
	}
	deadline := time.After(3 * time.Second)
	for !strings.Contains(collectText(store, "main", "loop"), "tick") {
		select {
		case <-deadline:
			t.Fatal("local process produced no output")
		case <-time.After(20 * time.Millisecond):
		}
	}
	if err := e.Stop(context.Background(), r, scheduler.Env{Worktree: "main"}); err != nil {
		t.Fatalf("stop: %v", err)
	}
	e.mu.Lock()
	n := len(e.running)
	e.mu.Unlock()
	if n != 0 {
		t.Errorf("process not cleaned up after stop, %d still tracked", n)
	}
}

func TestLocalRestartReplacesPrevious(t *testing.T) {
	e, _ := newTestEngine()
	r := localRes("svc", "sleep 30")
	env := scheduler.Env{Worktree: "main"}
	if err := e.Start(context.Background(), r, env, &nopReporter{}); err != nil {
		t.Fatal(err)
	}
	if err := e.Start(context.Background(), r, env, &nopReporter{}); err != nil {
		t.Fatal(err)
	}
	e.mu.Lock()
	n := len(e.running)
	e.mu.Unlock()
	if n != 1 {
		t.Errorf("restart should leave exactly 1 tracked process, got %d", n)
	}
	_ = e.Stop(context.Background(), r, env)
}

func TestLocalReportsRunning(t *testing.T) {
	e, _ := newTestEngine()
	r := localRes("svc", "sleep 30")
	rep := &nopReporter{}
	_ = e.Start(context.Background(), r, scheduler.Env{Worktree: "main"}, rep)
	defer func() { _ = e.Stop(context.Background(), r, scheduler.Env{Worktree: "main"}) }()
	if len(rep.phases) == 0 || rep.phases[len(rep.phases)-1] != scheduler.PhaseRunning {
		t.Errorf("expected running phase, got %v", rep.phases)
	}
}

func TestStopDoesNotReportFailure(t *testing.T) {
	e, _ := newTestEngine()
	r := localRes("svc", "sleep 30")
	rep := &nopReporter{}
	env := scheduler.Env{Worktree: "main"}
	if err := e.Start(context.Background(), r, env, rep); err != nil {
		t.Fatal(err)
	}
	if err := e.Stop(context.Background(), r, env); err != nil {
		t.Fatal(err)
	}
	// Give the exit-watch goroutine a moment to (incorrectly) fire if buggy.
	time.Sleep(100 * time.Millisecond)
	for _, p := range rep.phases {
		if p == scheduler.PhaseFailed {
			t.Errorf("intentional stop must not report failed; phases=%v", rep.phases)
		}
	}
}

func TestStopUntrackedIsNoop(t *testing.T) {
	e, _ := newTestEngine()
	if err := e.Stop(context.Background(), localRes("ghost", "x"), scheduler.Env{Worktree: "main"}); err != nil {
		t.Errorf("stopping untracked process should be no-op, got %v", err)
	}
}

func TestSampleRunningProcessReportsMemory(t *testing.T) {
	e, _ := newTestEngine()
	r := localRes("svc", "sleep 30")
	env := scheduler.Env{Worktree: "main"}
	if err := e.Start(context.Background(), r, env, &nopReporter{}); err != nil {
		t.Fatal(err)
	}
	defer func() { _ = e.Stop(context.Background(), r, env) }()

	u, ok := e.Sample(context.Background(), r, env)
	if !ok {
		t.Fatal("expected a sample for a running process")
	}
	if u.MemBytes == 0 {
		t.Error("running process should report non-zero RSS")
	}
}

func TestSampleUntrackedReturnsFalse(t *testing.T) {
	e, _ := newTestEngine()
	if _, ok := e.Sample(context.Background(), localRes("ghost", "x"), scheduler.Env{Worktree: "main"}); ok {
		t.Error("sampling a process that was never started should return ok=false")
	}
}

func TestSampleAfterStopReturnsFalse(t *testing.T) {
	e, _ := newTestEngine()
	r := localRes("svc", "sleep 30")
	env := scheduler.Env{Worktree: "main"}
	if err := e.Start(context.Background(), r, env, &nopReporter{}); err != nil {
		t.Fatal(err)
	}
	if err := e.Stop(context.Background(), r, env); err != nil {
		t.Fatal(err)
	}
	if _, ok := e.Sample(context.Background(), r, env); ok {
		t.Error("sampling after Stop should return ok=false")
	}
}

func TestExecRunsHostCommand(t *testing.T) {
	e, _ := newTestEngine()
	res, err := e.Exec(context.Background(), "main", "api",
		[]string{"sh", "-c", "echo out; echo err 1>&2"}, scheduler.Env{Worktree: "main"})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(res.Stdout, "out") {
		t.Errorf("stdout = %q", res.Stdout)
	}
	if !strings.Contains(res.Stderr, "err") {
		t.Errorf("stderr = %q", res.Stderr)
	}
	if res.ExitCode != 0 {
		t.Errorf("exit code = %d", res.ExitCode)
	}
}

func TestExecPropagatesEnv(t *testing.T) {
	e, _ := newTestEngine()
	res, err := e.Exec(context.Background(), "main", "api",
		[]string{"sh", "-c", "echo $FOO"}, scheduler.Env{Worktree: "main", Vars: map[string]string{"FOO": "barval"}})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(res.Stdout, "barval") {
		t.Errorf("env not propagated: %q", res.Stdout)
	}
}

func TestExecNonZeroExit(t *testing.T) {
	e, _ := newTestEngine()
	res, err := e.Exec(context.Background(), "main", "api",
		[]string{"sh", "-c", "exit 3"}, scheduler.Env{Worktree: "main"})
	if err != nil {
		t.Fatalf("non-zero exit should not be a hard error: %v", err)
	}
	if res.ExitCode != 3 {
		t.Errorf("exit code = %d, want 3", res.ExitCode)
	}
}

func TestExecEmptyCmd(t *testing.T) {
	e, _ := newTestEngine()
	if _, err := e.Exec(context.Background(), "main", "api", nil, scheduler.Env{}); err == nil {
		t.Error("empty command should error")
	}
}

func TestExecBadBinary(t *testing.T) {
	e, _ := newTestEngine()
	if _, err := e.Exec(context.Background(), "main", "api",
		[]string{"this-binary-does-not-exist-pando"}, scheduler.Env{}); err == nil {
		t.Error("nonexistent binary should error")
	}
}

func TestSyncIsNoop(t *testing.T) {
	e, _ := newTestEngine()
	if err := e.Sync(context.Background(), taskRes("a", "true"), scheduler.Env{}, "x", "y"); err != nil {
		t.Errorf("Sync stub should return nil, got %v", err)
	}
}
