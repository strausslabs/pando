package engine

import (
	"context"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/guyStrauss/pando/internal/api"
	"github.com/guyStrauss/pando/internal/executor"
	"github.com/guyStrauss/pando/internal/logbuf"
	"github.com/guyStrauss/pando/internal/resource"
	"github.com/guyStrauss/pando/internal/scheduler"
	"github.com/guyStrauss/pando/internal/state"
	"github.com/guyStrauss/pando/internal/worktree"
)

func testEngine(t *testing.T) (*Engine, *logbuf.Store, *state.Store) {
	t.Helper()
	logs := logbuf.NewStore(1000)
	store, err := state.Open(filepath.Join(t.TempDir(), "state.json"))
	if err != nil {
		t.Fatal(err)
	}
	proc := executor.NewEngine(logs, time.Now)
	eng := New(Config{
		StackName: "pando",
		Allocator: worktree.DefaultAllocator(),
		Store:     store,
		Logs:      logs,
		Executors: map[resource.Kind]scheduler.Executor{
			resource.KindTask:  proc,
			resource.KindLocal: proc,
		},
		Execers: map[resource.Kind]Execer{
			resource.KindLocal: proc,
			resource.KindTask:  proc,
		},
	})
	return eng, logs, store
}

func demoStack() *resource.Stack {
	return &resource.Stack{Name: "pando", Resources: []*resource.Resource{
		{Name: "setup", Kind: resource.KindTask, Task: &resource.TaskSpec{Cmd: "echo setting-up"}, RunWhen: resource.RunOnce},
		{Name: "worker", Kind: resource.KindLocal, Local: &resource.LocalSpec{Cmd: "echo started; sleep 30"}, Deps: []string{"setup"}},
	}}
}

func wt() worktree.Worktree {
	return worktree.Worktree{Path: "/tmp/demo", Branch: "main", Slug: "main"}
}

func TestEngineUpStatusDown(t *testing.T) {
	eng, _, _ := testEngine(t)
	if err := eng.Register(wt(), demoStack()); err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	if err := eng.Up(ctx, "main", false); err != nil {
		t.Fatalf("up: %v", err)
	}
	defer eng.Down(ctx, "main")

	st, err := eng.Status(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(st) != 1 {
		t.Fatalf("want 1 worktree, got %d", len(st))
	}
	phases := map[string]string{}
	for _, r := range st[0].Resources {
		phases[r.Name] = r.Phase
	}
	if phases["setup"] != string(scheduler.PhaseDone) {
		t.Errorf("setup should be done, got %q", phases["setup"])
	}
	if phases["worker"] != string(scheduler.PhaseRunning) && phases["worker"] != string(scheduler.PhaseHealthy) {
		t.Errorf("worker should be running/healthy, got %q", phases["worker"])
	}
}

func TestEngineRunOnceTaskSkipsSecondUp(t *testing.T) {
	eng, logs, _ := testEngine(t)
	_ = eng.Register(wt(), demoStack())
	ctx := context.Background()
	_ = eng.Up(ctx, "main", false)
	eng.Down(ctx, "main")

	firstCount := countLines(logs, "main", "setup", "setting-up")
	_ = eng.Up(ctx, "main", false)
	defer eng.Down(ctx, "main")
	secondCount := countLines(logs, "main", "setup", "setting-up")

	if secondCount != firstCount {
		t.Errorf("run-once setup task re-ran: %d -> %d", firstCount, secondCount)
	}
}

func TestEngineForceRerunsOnceTask(t *testing.T) {
	eng, logs, _ := testEngine(t)
	_ = eng.Register(wt(), demoStack())
	ctx := context.Background()
	_ = eng.Up(ctx, "main", false)
	eng.Down(ctx, "main")
	before := countLines(logs, "main", "setup", "setting-up")

	_ = eng.Up(ctx, "main", true)
	defer eng.Down(ctx, "main")
	after := countLines(logs, "main", "setup", "setting-up")
	if after <= before {
		t.Errorf("force should re-run once-task: %d -> %d", before, after)
	}
}

func TestEnginePortsDeterministicAndExposed(t *testing.T) {
	eng, _, _ := testEngine(t)
	stack := &resource.Stack{Name: "pando", Resources: []*resource.Resource{
		{Name: "api", Kind: resource.KindLocal, Local: &resource.LocalSpec{Cmd: "sleep 30"}},
	}}
	_ = eng.Register(wt(), stack)
	wts, _ := eng.ListWorktrees(context.Background())
	if len(wts) != 1 {
		t.Fatal("expected 1 worktree")
	}
	if _, ok := wts[0].Ports["api"]; !ok {
		t.Errorf("api port not allocated: %+v", wts[0].Ports)
	}
}

func TestEngineExecLocal(t *testing.T) {
	eng, _, _ := testEngine(t)
	stack := &resource.Stack{Name: "pando", Resources: []*resource.Resource{
		{Name: "api", Kind: resource.KindLocal, Local: &resource.LocalSpec{Cmd: "sleep 30"}},
	}}
	_ = eng.Register(wt(), stack)
	ctx := context.Background()
	_ = eng.Up(ctx, "main", false)
	defer eng.Down(ctx, "main")

	res, err := eng.Exec(ctx, apiExecReq())
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(res.Stdout, "pando-exec-ok") {
		t.Errorf("exec stdout wrong: %q", res.Stdout)
	}
}

func TestEngineUnknownWorktreeErrors(t *testing.T) {
	eng, _, _ := testEngine(t)
	if err := eng.Up(context.Background(), "ghost", false); err == nil {
		t.Error("up on unregistered worktree should error")
	}
}

func apiExecReq() api.ExecRequest {
	return api.ExecRequest{Worktree: "main", Resource: "api", Cmd: []string{"echo", "pando-exec-ok"}}
}

func waitForLine(logs *logbuf.Store, wt, res, substr string) bool {
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		if countLines(logs, wt, res, substr) > 0 {
			return true
		}
		time.Sleep(20 * time.Millisecond)
	}
	return false
}

func countLines(logs *logbuf.Store, wt, res, substr string) int {
	lines, _ := logs.Query(wt, res, logbuf.Query{})
	n := 0
	for _, l := range lines {
		if strings.Contains(l.Text, substr) {
			n++
		}
	}
	return n
}
