package watcher

import (
	"context"
	"sync"
	"testing"

	"github.com/guyStrauss/pando/internal/resource"
	"github.com/guyStrauss/pando/internal/worktree"
)

type fakeEngine struct {
	mu          sync.Mutex
	registered  map[string]*resource.Stack
	reloads     map[string]int
	deregisters []string
}

func newFakeEngine() *fakeEngine {
	return &fakeEngine{registered: map[string]*resource.Stack{}, reloads: map[string]int{}}
}

func (f *fakeEngine) Register(wt worktree.Worktree, stack *resource.Stack) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.registered[wt.Slug] = stack
	return nil
}
func (f *fakeEngine) Reload(ctx context.Context, slug string, next *resource.Stack) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.registered[slug] = next
	f.reloads[slug]++
	return nil
}
func (f *fakeEngine) Deregister(ctx context.Context, slug string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	delete(f.registered, slug)
	f.deregisters = append(f.deregisters, slug)
	return nil
}
func (f *fakeEngine) Registered(slug string) bool {
	f.mu.Lock()
	defer f.mu.Unlock()
	_, ok := f.registered[slug]
	return ok
}
func (f *fakeEngine) isRegistered(slug string) bool { return f.Registered(slug) }

type fakeLoader struct {
	mu     sync.Mutex
	stacks map[string]*resource.Stack // path -> stack
	err    error
}

func (l *fakeLoader) LoadFile(ctx context.Context, path string) (*resource.Stack, error) {
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.err != nil {
		return nil, l.err
	}
	if s, ok := l.stacks[path]; ok {
		return s, nil
	}
	return &resource.Stack{Name: "pando", Resources: []*resource.Resource{
		{Name: "api", Kind: resource.KindLocal, Local: &resource.LocalSpec{Cmd: "run"}},
	}}, nil
}

type fakeLister struct {
	mu  sync.Mutex
	wts []worktree.Worktree
}

func (l *fakeLister) set(wts []worktree.Worktree) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.wts = wts
}
func (l *fakeLister) List(ctx context.Context) ([]worktree.Worktree, error) {
	l.mu.Lock()
	defer l.mu.Unlock()
	return append([]worktree.Worktree(nil), l.wts...), nil
}

func newReconciler(t *testing.T, eng Engine, loader ConfigLoader, lister WorktreeLister) *Reconciler {
	t.Helper()
	r, err := NewReconciler(eng, loader, lister, "", Options{})
	if err != nil {
		t.Fatal(err)
	}
	return r
}

func wtree(slug string) worktree.Worktree {
	return worktree.Worktree{Path: "/tmp/" + slug, Branch: slug, Slug: slug}
}

func TestReconcileRegistersNewWorktrees(t *testing.T) {
	eng := newFakeEngine()
	lister := &fakeLister{}
	lister.set([]worktree.Worktree{wtree("main"), wtree("feat-x")})
	r := newReconciler(t, eng, &fakeLoader{}, lister)

	r.reconcileWorktrees(context.Background())

	if !eng.isRegistered("main") || !eng.isRegistered("feat-x") {
		t.Errorf("both worktrees should be registered: %v", eng.registered)
	}
}

func TestReconcileDeregistersVanished(t *testing.T) {
	eng := newFakeEngine()
	lister := &fakeLister{}
	lister.set([]worktree.Worktree{wtree("main"), wtree("feat-x")})
	r := newReconciler(t, eng, &fakeLoader{}, lister)
	r.reconcileWorktrees(context.Background())

	// feat-x removed.
	lister.set([]worktree.Worktree{wtree("main")})
	r.reconcileWorktrees(context.Background())

	if eng.isRegistered("feat-x") {
		t.Error("feat-x should be deregistered after it vanished")
	}
	if !eng.isRegistered("main") {
		t.Error("main should remain registered")
	}
}

func TestReconcileIdempotent(t *testing.T) {
	eng := newFakeEngine()
	lister := &fakeLister{}
	lister.set([]worktree.Worktree{wtree("main")})
	r := newReconciler(t, eng, &fakeLoader{}, lister)

	r.reconcileWorktrees(context.Background())
	r.reconcileWorktrees(context.Background())
	r.reconcileWorktrees(context.Background())

	eng.mu.Lock()
	defer eng.mu.Unlock()
	if len(eng.registered) != 1 {
		t.Errorf("repeated reconcile should not duplicate, got %d", len(eng.registered))
	}
}

func TestReloadConfigCallsEngineReload(t *testing.T) {
	eng := newFakeEngine()
	lister := &fakeLister{}
	lister.set([]worktree.Worktree{wtree("main")})
	r := newReconciler(t, eng, &fakeLoader{}, lister)
	r.reconcileWorktrees(context.Background())

	r.reloadConfig(context.Background(), "main")

	eng.mu.Lock()
	defer eng.mu.Unlock()
	if eng.reloads["main"] != 1 {
		t.Errorf("reloadConfig should trigger one engine Reload, got %d", eng.reloads["main"])
	}
}

func TestBrokenConfigDoesNotDeregister(t *testing.T) {
	eng := newFakeEngine()
	lister := &fakeLister{}
	lister.set([]worktree.Worktree{wtree("main")})
	loader := &fakeLoader{}
	r := newReconciler(t, eng, loader, lister)
	r.reconcileWorktrees(context.Background())

	// Config now fails to parse.
	loader.mu.Lock()
	loader.err = context.DeadlineExceeded
	loader.mu.Unlock()
	r.reloadConfig(context.Background(), "main")

	if !eng.isRegistered("main") {
		t.Error("broken config reload must keep the running stack registered")
	}
}
