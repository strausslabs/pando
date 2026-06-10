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

func wt2() worktree.Worktree {
	return worktree.Worktree{Path: "/tmp/demo2", Branch: "feat", Slug: "feat"}
}

// sharedStack has a daemon-level auth singleton plus a local api that depends on
// it (the AWS-auth pattern: one shared resource serving every worktree).
func sharedStack() *resource.Stack {
	return &resource.Stack{Name: "pando", Resources: []*resource.Resource{
		{Name: "auth", Kind: resource.KindTask, Task: &resource.TaskSpec{Cmd: "echo authed"}, Shared: true, RunWhen: resource.RunOnce},
		{Name: "api", Kind: resource.KindLocal, Local: &resource.LocalSpec{Cmd: "echo api-up; sleep 30"}, Deps: []string{"auth"}},
	}}
}

func findResource(st []api.WorktreeStatus, slug, name string) *api.ResourceStatus {
	for i := range st {
		if st[i].Worktree != slug {
			continue
		}
		for j := range st[i].Resources {
			if st[i].Resources[j].Name == name {
				return &st[i].Resources[j]
			}
		}
	}
	return nil
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

func TestEngineRestartRerunsSkippedOnceTask(t *testing.T) {
	eng, logs, _ := testEngine(t)
	_ = eng.Register(wt(), demoStack())
	ctx := context.Background()
	if err := eng.Up(ctx, "main", false); err != nil {
		t.Fatal(err)
	}
	defer eng.Down(ctx, "main")
	before := countLines(logs, "main", "setup", "setting-up")
	if before == 0 {
		t.Fatal("setup task never ran on first up")
	}

	// A second plain Up skips the run-once task; an explicit Restart must clear
	// its bookkeeping and run it again.
	if err := eng.Restart(ctx, "main", "setup"); err != nil {
		t.Fatalf("restart: %v", err)
	}
	if !waitForLine(logs, "main", "setup", "setting-up") {
		t.Fatal("no setup output after restart")
	}
	after := countLines(logs, "main", "setup", "setting-up")
	if after <= before {
		t.Errorf("restart did not re-run skipped once-task: %d -> %d", before, after)
	}
}

func TestEngineRestartUnknownResourceErrors(t *testing.T) {
	eng, _, _ := testEngine(t)
	_ = eng.Register(wt(), demoStack())
	if err := eng.Restart(context.Background(), "main", "ghost"); err == nil {
		t.Error("restart of unknown resource should error")
	}
}

func periodicStack(every time.Duration) *resource.Stack {
	return &resource.Stack{Name: "pando", Resources: []*resource.Resource{
		{Name: "sync", Kind: resource.KindTask, Task: &resource.TaskSpec{Cmd: "echo synced"}, Every: every},
	}}
}

func TestEnginePeriodicTaskReruns(t *testing.T) {
	eng, logs, _ := testEngine(t)
	_ = eng.Register(wt(), periodicStack(150*time.Millisecond))
	ctx := context.Background()
	if err := eng.Up(ctx, "main", false); err != nil {
		t.Fatal(err)
	}
	defer eng.Down(ctx, "main")

	// One run on Up plus at least two ticks within the budget.
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		if countLines(logs, "main", "sync", "synced") >= 3 {
			break
		}
		time.Sleep(20 * time.Millisecond)
	}
	if got := countLines(logs, "main", "sync", "synced"); got < 3 {
		t.Errorf("periodic task should have run >=3 times, got %d", got)
	}
}

func TestEngineDownStopsPeriodicLoop(t *testing.T) {
	eng, logs, _ := testEngine(t)
	_ = eng.Register(wt(), periodicStack(80*time.Millisecond))
	ctx := context.Background()
	_ = eng.Up(ctx, "main", false)
	if !waitForLine(logs, "main", "sync", "synced") {
		t.Fatal("periodic task never ran")
	}
	eng.Down(ctx, "main")
	settled := countLines(logs, "main", "sync", "synced")
	// After Down the ticker must be cancelled: no further runs accrue.
	time.Sleep(300 * time.Millisecond)
	if got := countLines(logs, "main", "sync", "synced"); got != settled {
		t.Errorf("periodic loop kept firing after Down: %d -> %d", settled, got)
	}
}

func TestEngineStatusReportsPeriodicSchedule(t *testing.T) {
	eng, _, _ := testEngine(t)
	_ = eng.Register(wt(), periodicStack(30*time.Minute))
	ctx := context.Background()
	_ = eng.Up(ctx, "main", false)
	defer eng.Down(ctx, "main")

	st, _ := eng.Status(ctx)
	var sync *api.ResourceStatus
	for i := range st[0].Resources {
		if st[0].Resources[i].Name == "sync" {
			sync = &st[0].Resources[i]
		}
	}
	if sync == nil {
		t.Fatal("sync resource missing from status")
	}
	if sync.EverySeconds != int64((30 * time.Minute).Seconds()) {
		t.Errorf("everySeconds wrong: %d", sync.EverySeconds)
	}
	if sync.NextRunUnix <= time.Now().Unix() {
		t.Errorf("nextRunUnix should be in the future, got %d", sync.NextRunUnix)
	}
}

func TestEngineStatusReportsLocalMemory(t *testing.T) {
	eng, _, _ := testEngine(t)
	stack := &resource.Stack{Name: "pando", Resources: []*resource.Resource{
		{Name: "svc", Kind: resource.KindLocal, Local: &resource.LocalSpec{Cmd: "sleep 30"}},
	}}
	_ = eng.Register(wt(), stack)
	ctx := context.Background()
	_ = eng.Up(ctx, "main", false)
	defer eng.Down(ctx, "main")

	st, _ := eng.Status(ctx)
	svc := st[0].Resources[0]
	if svc.MemBytes == 0 {
		t.Skip("sampler reported no RSS on this platform")
	}
}

func TestEngineStatusReportsPreview(t *testing.T) {
	eng, _, _ := testEngine(t)
	stack := &resource.Stack{Name: "pando", Resources: []*resource.Resource{
		{Name: "web", Kind: resource.KindLocal, Local: &resource.LocalSpec{Cmd: "sleep 30"}, Preview: true},
		{Name: "api", Kind: resource.KindLocal, Local: &resource.LocalSpec{Cmd: "sleep 30"}},
	}}
	_ = eng.Register(wt(), stack)
	ctx := context.Background()
	_ = eng.Up(ctx, "main", false)
	defer eng.Down(ctx, "main")

	st, _ := eng.Status(ctx)
	preview := map[string]bool{}
	for _, r := range st[0].Resources {
		preview[r.Name] = r.Preview
	}
	if !preview["web"] {
		t.Error("web should be flagged preview")
	}
	if preview["api"] {
		t.Error("api should not be flagged preview")
	}
}

func TestEngineSharedResourceHoistedAndDependentRuns(t *testing.T) {
	eng, logs, _ := testEngine(t)
	if err := eng.Register(wt(), sharedStack()); err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	if err := eng.Up(ctx, "main", false); err != nil {
		t.Fatalf("up: %v", err)
	}
	defer eng.Down(ctx, "main")
	defer eng.Down(ctx, sharedSlug)

	st, _ := eng.Status(ctx)
	// auth lives in the shared stack, not the worktree.
	if findResource(st, "main", "auth") != nil {
		t.Error("shared 'auth' should not appear under the worktree")
	}
	auth := findResource(st, sharedSlug, "auth")
	if auth == nil {
		t.Fatal("shared 'auth' missing from shared stack")
	}
	if auth.Phase != string(scheduler.PhaseDone) {
		t.Errorf("auth phase = %q, want done", auth.Phase)
	}
	// api depended on auth and should have come up once auth was ready.
	if !waitForLine(logs, "main", "api", "api-up") {
		t.Error("api (depends on shared auth) never started")
	}
}

func TestEngineSharedSingletonAcrossWorktrees(t *testing.T) {
	eng, _, _ := testEngine(t)
	_ = eng.Register(wt(), sharedStack())
	_ = eng.Register(wt2(), sharedStack())
	ctx := context.Background()
	_ = eng.Up(ctx, "main", false)
	_ = eng.Up(ctx, "feat", false)
	defer eng.Down(ctx, "main")
	defer eng.Down(ctx, "feat")
	defer eng.Down(ctx, sharedSlug)

	st, _ := eng.Status(ctx)
	// Exactly one shared stack, with exactly one auth resource, despite two
	// worktrees declaring it.
	count := 0
	for _, w := range st {
		if w.Worktree == sharedSlug {
			for _, r := range w.Resources {
				if r.Name == "auth" {
					count++
				}
			}
		}
	}
	if count != 1 {
		t.Errorf("shared auth should be a single singleton, found %d", count)
	}
}

func TestEngineSharedMayNotDependOnLocal(t *testing.T) {
	eng, _, _ := testEngine(t)
	// A shared resource depending on a non-shared resource must be rejected.
	bad := &resource.Stack{Name: "pando", Resources: []*resource.Resource{
		{Name: "local-db", Kind: resource.KindLocal, Local: &resource.LocalSpec{Cmd: "sleep 30"}},
		{Name: "auth", Kind: resource.KindTask, Task: &resource.TaskSpec{Cmd: "echo x"}, Shared: true, Deps: []string{"local-db"}},
	}}
	if err := eng.Register(wt(), bad); err == nil {
		t.Error("shared resource depending on a local resource should error")
	}
}

func TestTriggeredMatching(t *testing.T) {
	root := "/work"
	cases := []struct {
		name     string
		triggers []string
		changed  []string
		want     bool
	}{
		{"no trigger always fires", nil, []string{"/work/src/a.go"}, true},
		{"basename match", []string{"requirements.txt"}, []string{"/work/requirements.txt"}, true},
		{"recursive glob match", []string{"**/*.go"}, []string{"/work/src/pkg/a.go"}, true},
		{"relative glob match", []string{"src/*.go"}, []string{"/work/src/a.go"}, true},
		{"no match", []string{"*.py"}, []string{"/work/src/a.go"}, false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := triggered(c.triggers, c.changed, root); got != c.want {
				t.Errorf("triggered(%v, %v) = %v, want %v", c.triggers, c.changed, got, c.want)
			}
		})
	}
}

func TestRunLiveUpdateRunStepGatedByTrigger(t *testing.T) {
	eng, logs, _ := testEngine(t)
	r := &resource.Resource{
		Name: "api", Kind: resource.KindLocal,
		Local: &resource.LocalSpec{Cmd: "sleep 30"},
		LiveUpdate: []resource.LiveUpdateStep{
			{Run: "echo rebuilt", Trigger: []string{"*.go"}},
		},
	}
	_ = eng.Register(wt(), &resource.Stack{Name: "pando", Resources: []*resource.Resource{r}})
	ctx := context.Background()
	_ = eng.Up(ctx, "main", false)
	defer eng.Down(ctx, "main")
	as, _ := eng.lookup("main")
	lr, _ := as.stack.Get("api")

	// A non-matching change must NOT run the step.
	_ = eng.runLiveUpdate(ctx, as, lr, []string{"/tmp/demo/readme.md"})
	if countLines(logs, "main", "api", "rebuilt") != 0 {
		t.Error("run step fired for a non-matching change")
	}
	// A matching change runs it.
	_ = eng.runLiveUpdate(ctx, as, lr, []string{"/tmp/demo/main.go"})
	if !waitForLine(logs, "main", "api", "rebuilt") {
		t.Error("run step did not fire for a matching change")
	}
}

func TestRunLiveUpdateRestartReruns(t *testing.T) {
	eng, logs, _ := testEngine(t)
	r := &resource.Resource{
		Name: "api", Kind: resource.KindLocal,
		Local:      &resource.LocalSpec{Cmd: "echo booting; sleep 30"},
		LiveUpdate: []resource.LiveUpdateStep{{Restart: true}},
	}
	_ = eng.Register(wt(), &resource.Stack{Name: "pando", Resources: []*resource.Resource{r}})
	ctx := context.Background()
	_ = eng.Up(ctx, "main", false)
	defer eng.Down(ctx, "main")
	if !waitForLine(logs, "main", "api", "booting") {
		t.Fatal("api never booted")
	}
	before := countLines(logs, "main", "api", "booting")
	as, _ := eng.lookup("main")
	lr, _ := as.stack.Get("api")
	if err := eng.runLiveUpdate(ctx, as, lr, []string{"/tmp/demo/main.go"}); err != nil {
		t.Fatalf("live update: %v", err)
	}
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		if countLines(logs, "main", "api", "booting") > before {
			return
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Errorf("restart step did not re-run the process: still %d boots", before)
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
