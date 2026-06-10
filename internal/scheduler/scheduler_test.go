package scheduler

import (
	"context"
	"fmt"
	"sync"
	"testing"

	"github.com/guyStrauss/pando/internal/dag"
	"github.com/guyStrauss/pando/internal/resource"
)

type fakeExec struct {
	mu       sync.Mutex
	started  []string
	stopped  []string
	failOn   map[string]bool
	startedC map[string]int
}

func newFakeExec() *fakeExec {
	return &fakeExec{failOn: map[string]bool{}, startedC: map[string]int{}}
}

func (f *fakeExec) Start(ctx context.Context, r *resource.Resource, env Env, rep Reporter) error {
	f.mu.Lock()
	f.started = append(f.started, r.Name)
	f.startedC[r.Name]++
	fail := f.failOn[r.Name]
	f.mu.Unlock()
	if fail {
		return fmt.Errorf("forced failure on %s", r.Name)
	}
	return nil
}

func (f *fakeExec) Stop(ctx context.Context, r *resource.Resource, env Env) error {
	f.mu.Lock()
	f.stopped = append(f.stopped, r.Name)
	f.mu.Unlock()
	return nil
}

func (f *fakeExec) startCount(name string) int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.startedC[name]
}

type memStore struct {
	mu     sync.Mutex
	ran    map[string]bool
	inputs map[string]string
}

func newMemStore() *memStore {
	return &memStore{ran: map[string]bool{}, inputs: map[string]string{}}
}

func key(wt, res string) string { return wt + "/" + res }

func (m *memStore) HasRun(wt, res string) bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.ran[key(wt, res)]
}
func (m *memStore) MarkRun(wt, res string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.ran[key(wt, res)] = true
}
func (m *memStore) LastInputs(wt, res string) string {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.inputs[key(wt, res)]
}
func (m *memStore) SetInputs(wt, res, h string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.inputs[key(wt, res)] = h
}

func task(name string, deps ...string) *resource.Resource {
	return &resource.Resource{Name: name, Kind: resource.KindTask, Task: &resource.TaskSpec{Cmd: "x"}, Deps: deps}
}

func svc(name string, deps ...string) *resource.Resource {
	return &resource.Resource{Name: name, Kind: resource.KindLocal, Local: &resource.LocalSpec{Cmd: "run"}, Deps: deps}
}

func graph(t *testing.T, rs ...*resource.Resource) *dag.Graph {
	t.Helper()
	g, err := dag.Build(&resource.Stack{Name: "s", Resources: rs})
	if err != nil {
		t.Fatal(err)
	}
	return g
}

func TestUpAllHealthy(t *testing.T) {
	g := graph(t, svc("db"), task("migrate", "db"), svc("api", "migrate"))
	fe := newFakeExec()
	s := New(g, Options{Executors: map[resource.Kind]Executor{
		resource.KindLocal: fe, resource.KindTask: fe,
	}})
	if err := s.Up(context.Background()); err != nil {
		t.Fatalf("up: %v", err)
	}
	if s.Phase("db") != PhaseHealthy {
		t.Errorf("db should be healthy, got %s", s.Phase("db"))
	}
	if s.Phase("migrate") != PhaseDone {
		t.Errorf("migrate (task) should be done, got %s", s.Phase("migrate"))
	}
	if s.Phase("api") != PhaseHealthy {
		t.Errorf("api should be healthy, got %s", s.Phase("api"))
	}
}

func TestUpRespectsDependencyOrder(t *testing.T) {
	g := graph(t, svc("db"), task("migrate", "db"), svc("api", "migrate"))
	fe := newFakeExec()
	s := New(g, Options{Executors: map[resource.Kind]Executor{
		resource.KindLocal: fe, resource.KindTask: fe,
	}})
	_ = s.Up(context.Background())
	pos := map[string]int{}
	for i, n := range fe.started {
		pos[n] = i
	}
	if pos["db"] > pos["migrate"] || pos["migrate"] > pos["api"] {
		t.Errorf("start order violated deps: %v", fe.started)
	}
}

func TestFailureBlocksDependents(t *testing.T) {
	g := graph(t, svc("db"), task("migrate", "db"), svc("api", "migrate"), svc("frontend"))
	fe := newFakeExec()
	fe.failOn["migrate"] = true
	s := New(g, Options{Executors: map[resource.Kind]Executor{
		resource.KindLocal: fe, resource.KindTask: fe,
	}})
	err := s.Up(context.Background())
	if err == nil {
		t.Fatal("expected error from failed migrate")
	}
	if s.Phase("migrate") != PhaseFailed {
		t.Errorf("migrate should be failed, got %s", s.Phase("migrate"))
	}
	if s.Phase("api") != PhaseBlocked {
		t.Errorf("api should be blocked by failed migrate, got %s", s.Phase("api"))
	}
	if s.Phase("frontend") != PhaseHealthy {
		t.Errorf("frontend independent, should be healthy, got %s", s.Phase("frontend"))
	}
}

func TestRunOnceSkippedSecondTime(t *testing.T) {
	g := graph(t, task("migrate"))
	fe := newFakeExec()
	store := newMemStore()
	mk := func() *Scheduler {
		return New(g, Options{
			Executors: map[resource.Kind]Executor{resource.KindTask: fe},
			Store:     store,
			Env:       Env{Worktree: "main"},
		})
	}
	_ = mk().Up(context.Background())
	_ = mk().Up(context.Background())
	if c := fe.startCount("migrate"); c != 1 {
		t.Errorf("run-once task should start exactly once, started %d times", c)
	}
}

func TestRunOnceIsolatedPerWorktree(t *testing.T) {
	g := graph(t, task("migrate"))
	fe := newFakeExec()
	store := newMemStore()
	for _, wt := range []string{"main", "feat-x"} {
		s := New(g, Options{
			Executors: map[resource.Kind]Executor{resource.KindTask: fe},
			Store:     store,
			Env:       Env{Worktree: wt},
		})
		_ = s.Up(context.Background())
	}
	if c := fe.startCount("migrate"); c != 2 {
		t.Errorf("run-once should run once per worktree (2 total), got %d", c)
	}
}

func TestOnChangeSkipsWhenInputUnchanged(t *testing.T) {
	r := &resource.Resource{
		Name: "seed", Kind: resource.KindTask, Task: &resource.TaskSpec{Cmd: "seed"},
		RunWhen: resource.RunOnChange, OnChange: []string{"./seed"},
	}
	g := graph(t, r)
	fe := newFakeExec()
	store := newMemStore()
	hash := "abc123"
	mk := func() *Scheduler {
		return New(g, Options{
			Executors: map[resource.Kind]Executor{resource.KindTask: fe},
			Store:     store,
			Env:       Env{Worktree: "main"},
			InputHash: func(*resource.Resource) string { return hash },
		})
	}
	_ = mk().Up(context.Background())
	_ = mk().Up(context.Background())
	if c := fe.startCount("seed"); c != 1 {
		t.Errorf("unchanged input should skip, started %d times", c)
	}
	hash = "def456"
	_ = mk().Up(context.Background())
	if c := fe.startCount("seed"); c != 2 {
		t.Errorf("changed input should re-run, started %d times", c)
	}
}

func TestDownReverseOrder(t *testing.T) {
	g := graph(t, svc("db"), svc("api", "db"))
	fe := newFakeExec()
	s := New(g, Options{Executors: map[resource.Kind]Executor{resource.KindLocal: fe}})
	_ = s.Up(context.Background())
	_ = s.Down(context.Background())
	pos := map[string]int{}
	for i, n := range fe.stopped {
		pos[n] = i
	}
	if pos["api"] > pos["db"] {
		t.Errorf("api (dependent) must stop before db: %v", fe.stopped)
	}
}

func TestUpSubsetOnlyTouchesDirty(t *testing.T) {
	g := graph(t, svc("db"), task("migrate", "db"), svc("api", "migrate"), svc("frontend"))
	fe := newFakeExec()
	s := New(g, Options{Executors: map[resource.Kind]Executor{
		resource.KindLocal: fe, resource.KindTask: fe,
	}})
	_ = s.Up(context.Background())
	// db, migrate, api healthy. Now only api changed -> only api restarts.
	before := fe.startCount("db")
	_ = s.UpSubset(context.Background(), "api")
	if fe.startCount("db") != before {
		t.Error("db should not restart on api change")
	}
	if fe.startCount("api") != 2 {
		t.Errorf("api should restart once on its own change, got %d", fe.startCount("api"))
	}
}

func TestStateCallbackFires(t *testing.T) {
	g := graph(t, svc("api"))
	fe := newFakeExec()
	var mu sync.Mutex
	phases := []Phase{}
	s := New(g, Options{
		Executors: map[resource.Kind]Executor{resource.KindLocal: fe},
		OnState: func(ns NodeState) {
			mu.Lock()
			phases = append(phases, ns.Phase)
			mu.Unlock()
		},
	})
	_ = s.Up(context.Background())
	mu.Lock()
	defer mu.Unlock()
	if len(phases) < 2 {
		t.Fatalf("expected starting+healthy callbacks, got %v", phases)
	}
	if phases[len(phases)-1] != PhaseHealthy {
		t.Errorf("last phase should be healthy, got %v", phases)
	}
}
