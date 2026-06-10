package scheduler

import (
	"context"
	"fmt"
	"sync"

	"github.com/guyStrauss/pando/internal/dag"
	"github.com/guyStrauss/pando/internal/resource"
)

type NodeState struct {
	Name  string
	Phase Phase
	Err   error
}

type StateFunc func(NodeState)

// RunStore persists run-once / onChange bookkeeping across daemon restarts so a
// migrate task does not re-run when the user switches away and back.
type RunStore interface {
	HasRun(worktree, resource string) bool
	MarkRun(worktree, resource string)
	LastInputs(worktree, resource string) string
	SetInputs(worktree, resource, hash string)
}

type Scheduler struct {
	graph     *dag.Graph
	execs     map[resource.Kind]Executor
	store     RunStore
	env       Env
	onState   StateFunc
	inputHash func(*resource.Resource) string
	waitReady func(ctx context.Context, r *resource.Resource, env Env) error

	mu     sync.Mutex
	states map[string]Phase
}

type Options struct {
	Executors map[resource.Kind]Executor
	Store     RunStore
	Env       Env
	OnState   StateFunc
	// InputHash computes a content hash for a resource's onChange inputs. May be
	// nil, in which case onChange resources always run.
	InputHash func(*resource.Resource) string
	// WaitReady blocks until the resource's readiness probe passes. Called for
	// long-running resources after Start returns; nil means skip probing.
	WaitReady func(ctx context.Context, r *resource.Resource, env Env) error
}

func New(g *dag.Graph, opts Options) *Scheduler {
	onState := opts.OnState
	if onState == nil {
		onState = func(NodeState) {}
	}
	return &Scheduler{
		graph:     g,
		execs:     opts.Executors,
		store:     opts.Store,
		env:       opts.Env,
		onState:   onState,
		inputHash: opts.InputHash,
		waitReady: opts.WaitReady,
		states:    make(map[string]Phase),
	}
}

func (s *Scheduler) Phase(name string) Phase {
	s.mu.Lock()
	defer s.mu.Unlock()
	if p, ok := s.states[name]; ok {
		return p
	}
	return PhasePending
}

func (s *Scheduler) set(name string, p Phase, err error) {
	s.mu.Lock()
	s.states[name] = p
	s.mu.Unlock()
	s.onState(NodeState{Name: name, Phase: p, Err: err})
}

// Seed pre-populates phase state without notifying or executing. Used when a
// stack is reloaded: resources that are unchanged and already healthy carry
// their phase into the new scheduler so a partial re-run treats them as
// satisfied dependencies.
func (s *Scheduler) Seed(phases map[string]Phase) {
	s.mu.Lock()
	defer s.mu.Unlock()
	for name, p := range phases {
		s.states[name] = p
	}
}

// Up brings the whole graph to a settled state in topological order, running
// independent branches concurrently. A dependency that fails or is blocked
// transitively blocks its dependents. Returns the first fatal error, if any.
func (s *Scheduler) Up(ctx context.Context) error {
	return s.run(ctx, s.graph.TopoOrder())
}

// UpSubset runs only the given changed nodes and their transitive dependents,
// reusing already-healthy dependencies. Used by the file watcher.
func (s *Scheduler) UpSubset(ctx context.Context, changed ...string) error {
	return s.run(ctx, s.graph.Dirty(changed...))
}

func (s *Scheduler) run(ctx context.Context, names []string) error {
	results := make(map[string]Phase, len(names))
	var mu sync.Mutex
	done := make(map[string]chan struct{}, len(names))
	for _, name := range names {
		done[name] = make(chan struct{})
	}

	var wg sync.WaitGroup
	var firstErr error
	var errMu sync.Mutex

	for _, name := range names {
		name := name
		wg.Add(1)
		go func() {
			defer wg.Done()
			defer close(done[name])

			blocked := false
			for _, dep := range s.graph.Deps(name) {
				if ch, ok := done[dep]; ok {
					select {
					case <-ch:
					case <-ctx.Done():
						s.set(name, PhaseStopped, ctx.Err())
						mu.Lock()
						results[name] = PhaseStopped
						mu.Unlock()
						return
					}
					mu.Lock()
					depPhase := results[dep]
					mu.Unlock()
					if !depPhase.OK() {
						blocked = true
						break
					}
				} else {
					// Dep outside this run set; require it already healthy.
					if !s.Phase(dep).OK() {
						blocked = true
						break
					}
				}
			}

			var phase Phase
			if blocked {
				phase = PhaseBlocked
				s.set(name, PhaseBlocked, nil)
			} else {
				phase = s.startOne(ctx, name)
			}
			mu.Lock()
			results[name] = phase
			mu.Unlock()
			if phase == PhaseFailed {
				errMu.Lock()
				if firstErr == nil {
					firstErr = fmt.Errorf("resource %q failed", name)
				}
				errMu.Unlock()
			}
		}()
	}

	wg.Wait()
	return firstErr
}

func (s *Scheduler) startOne(ctx context.Context, name string) Phase {
	node, _ := s.graph.Node(name)
	r := node.Resource

	if skip := s.shouldSkip(r); skip {
		s.set(name, PhaseSkipped, nil)
		return PhaseSkipped
	}

	exec, ok := s.execs[r.Kind]
	if !ok {
		err := fmt.Errorf("no executor for kind %q", r.Kind)
		s.set(name, PhaseFailed, err)
		return PhaseFailed
	}

	s.set(name, PhaseStarting, nil)
	rep := &reporter{s: s, name: name}
	if err := exec.Start(ctx, r, s.env, rep); err != nil {
		s.set(name, PhaseFailed, err)
		return PhaseFailed
	}

	// Long-running resources only count as healthy once their probe passes, so
	// dependents do not start against a not-yet-listening service.
	if r.Kind != resource.KindTask && s.waitReady != nil && r.Ready.Kind != resource.ProbeNone {
		if err := s.waitReady(ctx, r, s.env); err != nil {
			s.set(name, PhaseFailed, err)
			return PhaseFailed
		}
	}

	s.recordRun(r)

	final := s.terminalFor(r)
	s.set(name, final, nil)
	return final
}

// shouldSkip applies runWhen policy. once: skip if already run this worktree.
// onChange: skip if input hash unchanged since last run.
func (s *Scheduler) shouldSkip(r *resource.Resource) bool {
	if s.store == nil {
		return false
	}
	switch r.DefaultRunPolicy() {
	case resource.RunOnce:
		return s.store.HasRun(s.env.Worktree, r.Name)
	case resource.RunOnChange:
		if s.inputHash == nil {
			return false
		}
		cur := s.inputHash(r)
		return cur != "" && cur == s.store.LastInputs(s.env.Worktree, r.Name)
	default:
		return false
	}
}

func (s *Scheduler) recordRun(r *resource.Resource) {
	if s.store == nil {
		return
	}
	switch r.DefaultRunPolicy() {
	case resource.RunOnce:
		s.store.MarkRun(s.env.Worktree, r.Name)
	case resource.RunOnChange:
		if s.inputHash != nil {
			s.store.SetInputs(s.env.Worktree, r.Name, s.inputHash(r))
		}
	}
}

func (s *Scheduler) terminalFor(r *resource.Resource) Phase {
	if r.Kind == resource.KindTask {
		return PhaseDone
	}
	return PhaseHealthy
}

func (s *Scheduler) Down(ctx context.Context) error {
	order := s.graph.TopoOrder()
	var firstErr error
	// Reverse topological order so dependents stop before their deps.
	for i := len(order) - 1; i >= 0; i-- {
		name := order[i]
		node, _ := s.graph.Node(name)
		r := node.Resource
		exec, ok := s.execs[r.Kind]
		if !ok {
			continue
		}
		if err := exec.Stop(ctx, r, s.env); err != nil && firstErr == nil {
			firstErr = err
		}
		s.set(name, PhaseStopped, nil)
	}
	return firstErr
}

type reporter struct {
	s    *Scheduler
	name string
}

func (r *reporter) Phase(p Phase)                { r.s.set(r.name, p, nil) }
func (r *reporter) Logf(format string, a ...any) { _ = fmt.Sprintf(format, a...) }
