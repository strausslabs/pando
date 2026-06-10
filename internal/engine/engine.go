package engine

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/guyStrauss/pando/internal/api"
	"github.com/guyStrauss/pando/internal/dag"
	"github.com/guyStrauss/pando/internal/interp"
	"github.com/guyStrauss/pando/internal/logbuf"
	"github.com/guyStrauss/pando/internal/probe"
	"github.com/guyStrauss/pando/internal/resource"
	"github.com/guyStrauss/pando/internal/scheduler"
	"github.com/guyStrauss/pando/internal/worktree"
)

// Execer runs a one-shot command inside a started resource (container or host
// process) for the Exec API. Backends implement it; the process and compose
// executors both satisfy it.
type Execer interface {
	Exec(ctx context.Context, worktree, resource string, cmd []string, env scheduler.Env) (api.ExecResult, error)
}

type RunStore interface {
	scheduler.RunStore
	Reset(worktree string)
}

type Config struct {
	StackName string
	Allocator worktree.PortAllocator
	Store     RunStore
	Logs      *logbuf.Store
	Executors map[resource.Kind]scheduler.Executor
	Execers   map[resource.Kind]Execer
	Clock     func() time.Time
}

// Engine owns the live state for every active worktree: its compiled graph,
// scheduler, resolved ports, and last-known phases. It implements api.StackOps.
type Engine struct {
	cfg Config

	mu     sync.RWMutex
	stacks map[string]*activeStack
}

type activeStack struct {
	info   api.WorktreeInfo
	stack  *resource.Stack
	graph  *dag.Graph
	sched  *scheduler.Scheduler
	env    scheduler.Env
	phases map[string]scheduler.Phase
	errs   map[string]string
}

func New(cfg Config) *Engine {
	if cfg.Clock == nil {
		cfg.Clock = time.Now
	}
	return &Engine{cfg: cfg, stacks: map[string]*activeStack{}}
}

// Register compiles a stack for a worktree and prepares its scheduler without
// starting anything. Ports are allocated deterministically from the worktree
// path and the resource set.
func (e *Engine) Register(wt worktree.Worktree, stack *resource.Stack) error {
	g, err := dag.Build(stack)
	if err != nil {
		return err
	}
	ports := e.cfg.Allocator.Allocate(wt.Path, resourceNames(stack))
	env := scheduler.Env{
		Worktree: wt.Slug,
		Project:  worktree.ProjectName(e.cfg.StackName, wt.Slug),
		Ports:    ports,
		Vars:     map[string]string{},
	}

	as := &activeStack{
		info:   api.WorktreeInfo{Path: wt.Path, Branch: wt.Branch, Slug: wt.Slug, Ports: ports},
		stack:  stack,
		graph:  g,
		env:    env,
		phases: map[string]scheduler.Phase{},
		errs:   map[string]string{},
	}
	as.sched = e.newScheduler(wt.Slug, g, env, as)

	e.mu.Lock()
	e.stacks[wt.Slug] = as
	e.mu.Unlock()
	return nil
}

func (e *Engine) newScheduler(slug string, g *dag.Graph, env scheduler.Env, as *activeStack) *scheduler.Scheduler {
	return scheduler.New(g, scheduler.Options{
		Executors: e.cfg.Executors,
		Store:     e.cfg.Store,
		Env:       env,
		OnState:   e.stateHandler(slug, as),
		WaitReady: e.waitReady,
	})
}

func resourceNames(stack *resource.Stack) []string {
	names := make([]string, 0, len(stack.Resources))
	for _, r := range stack.Resources {
		names = append(names, r.Name)
	}
	return names
}

// Reload swaps in a new stack definition for an already-registered worktree and
// re-runs only what changed. Removed resources are stopped; unchanged healthy
// resources keep running and seed the new scheduler; added and changed
// resources (plus their dependents) are re-run. This is the surgical config
// hot-reload: no full teardown.
func (e *Engine) Reload(ctx context.Context, slug string, next *resource.Stack) error {
	e.mu.Lock()
	as, ok := e.stacks[slug]
	if !ok {
		e.mu.Unlock()
		return fmt.Errorf("worktree %q not registered", slug)
	}
	old := as.stack
	oldPhases := copyPhases(as.phases)
	info := as.info
	e.mu.Unlock()

	g, err := dag.Build(next)
	if err != nil {
		return err
	}
	diff := resource.DiffStacks(old, next)

	// Stop resources that no longer exist, before swapping the graph.
	for _, name := range diff.Removed {
		if r, found := old.Get(name); found {
			if exec, ok := e.cfg.Executors[r.Kind]; ok {
				_ = exec.Stop(ctx, r, as.env)
			}
		}
	}

	ports := e.cfg.Allocator.Allocate(info.Path, resourceNames(next))
	env := as.env
	env.Ports = ports

	newAS := &activeStack{
		info:   api.WorktreeInfo{Path: info.Path, Branch: info.Branch, Slug: slug, Ports: ports},
		stack:  next,
		graph:  g,
		env:    env,
		phases: map[string]scheduler.Phase{},
		errs:   map[string]string{},
	}
	newAS.sched = e.newScheduler(slug, g, env, newAS)

	// Carry forward phases for resources that survive unchanged so their
	// dependents see them as satisfied without re-running.
	changed := markSet(diff.Added, diff.Changed)
	seed := map[string]scheduler.Phase{}
	for _, r := range next.Resources {
		if changed[r.Name] {
			continue
		}
		if p, ok := oldPhases[r.Name]; ok {
			seed[r.Name] = p
			newAS.phases[r.Name] = p
		}
	}
	newAS.sched.Seed(seed)

	e.mu.Lock()
	e.stacks[slug] = newAS
	e.mu.Unlock()

	dirty := append(append([]string{}, diff.Added...), diff.Changed...)
	if len(dirty) == 0 {
		return nil
	}
	return newAS.sched.UpSubset(ctx, dirty...)
}

// Deregister stops everything for a worktree and forgets it. Used when a git
// worktree is removed.
func (e *Engine) Deregister(ctx context.Context, slug string) error {
	e.mu.Lock()
	as, ok := e.stacks[slug]
	if ok {
		delete(e.stacks, slug)
	}
	e.mu.Unlock()
	if !ok {
		return nil
	}
	return as.sched.Down(ctx)
}

func (e *Engine) Registered(slug string) bool {
	e.mu.RLock()
	defer e.mu.RUnlock()
	_, ok := e.stacks[slug]
	return ok
}

func copyPhases(in map[string]scheduler.Phase) map[string]scheduler.Phase {
	out := make(map[string]scheduler.Phase, len(in))
	for k, v := range in {
		out[k] = v
	}
	return out
}

func markSet(lists ...[]string) map[string]bool {
	m := map[string]bool{}
	for _, l := range lists {
		for _, s := range l {
			m[s] = true
		}
	}
	return m
}

func (e *Engine) stateHandler(slug string, as *activeStack) scheduler.StateFunc {
	return func(ns scheduler.NodeState) {
		e.mu.Lock()
		as.phases[ns.Name] = ns.Phase
		if ns.Err != nil {
			as.errs[ns.Name] = ns.Err.Error()
		} else {
			delete(as.errs, ns.Name)
		}
		e.mu.Unlock()
		if e.cfg.Logs != nil {
			e.cfg.Logs.PublishPhase(slug, ns.Name, string(ns.Phase))
		}
	}
}

func (e *Engine) waitReady(ctx context.Context, r *resource.Resource, env scheduler.Env) error {
	return probe.Wait(ctx, r.Ready, probe.Options{
		Scope:    interp.Scope{Ports: env.Ports, Vars: env.Vars},
		Worktree: env.Worktree,
		Resource: r.Name,
		Logs:     e.cfg.Logs,
	})
}

func (e *Engine) lookup(slug string) (*activeStack, error) {
	e.mu.RLock()
	defer e.mu.RUnlock()
	as, ok := e.stacks[slug]
	if !ok {
		return nil, fmt.Errorf("worktree %q not registered", slug)
	}
	return as, nil
}

func (e *Engine) Up(ctx context.Context, slug string, force bool) error {
	as, err := e.lookup(slug)
	if err != nil {
		return err
	}
	if force {
		e.cfg.Store.Reset(slug)
	}
	return as.sched.Up(ctx)
}

func (e *Engine) Down(ctx context.Context, slug string) error {
	as, err := e.lookup(slug)
	if err != nil {
		return err
	}
	return as.sched.Down(ctx)
}

func (e *Engine) Restart(ctx context.Context, slug, name string) error {
	as, err := e.lookup(slug)
	if err != nil {
		return err
	}
	if _, ok := as.stack.Get(name); !ok {
		return fmt.Errorf("resource %q not found in worktree %q", name, slug)
	}
	return as.sched.UpSubset(ctx, name)
}

func (e *Engine) Rebuild(ctx context.Context, slug, name string) error {
	return e.Restart(ctx, slug, name)
}

func (e *Engine) Trigger(ctx context.Context, slug, name string) error {
	return e.Restart(ctx, slug, name)
}

func (e *Engine) ListWorktrees(ctx context.Context) ([]api.WorktreeInfo, error) {
	e.mu.RLock()
	defer e.mu.RUnlock()
	out := make([]api.WorktreeInfo, 0, len(e.stacks))
	for _, as := range e.stacks {
		out = append(out, as.info)
	}
	return out, nil
}

func (e *Engine) Status(ctx context.Context) ([]api.WorktreeStatus, error) {
	e.mu.RLock()
	defer e.mu.RUnlock()
	out := make([]api.WorktreeStatus, 0, len(e.stacks))
	for slug, as := range e.stacks {
		ws := api.WorktreeStatus{Worktree: slug, Branch: as.info.Branch}
		for _, r := range as.stack.Resources {
			phase := as.phases[r.Name]
			ws.Resources = append(ws.Resources, api.ResourceStatus{
				Name:  r.Name,
				Kind:  string(r.Kind),
				Phase: string(phase),
				Ready: phase.OK(),
				Port:  as.info.Ports[r.Name],
				Error: as.errs[r.Name],
			})
		}
		out = append(out, ws)
	}
	return out, nil
}

func (e *Engine) Logs(ctx context.Context, q api.LogQuery) ([]api.LogLine, error) {
	if e.cfg.Logs == nil {
		return nil, nil
	}
	lines, err := e.cfg.Logs.Query(q.Worktree, q.Resource, logbuf.Query{
		Tail:  q.Tail,
		Since: q.Since,
		Grep:  q.Grep,
	})
	if err != nil {
		return nil, err
	}
	out := make([]api.LogLine, len(lines))
	for i, l := range lines {
		out[i] = api.LogLine{
			Seq:      l.Seq,
			Time:     l.Time,
			Worktree: l.Worktree,
			Resource: l.Resource,
			Stream:   string(l.Stream),
			Text:     l.Text,
		}
	}
	return out, nil
}

func (e *Engine) Exec(ctx context.Context, req api.ExecRequest) (api.ExecResult, error) {
	as, err := e.lookup(req.Worktree)
	if err != nil {
		return api.ExecResult{}, err
	}
	r, ok := as.stack.Get(req.Resource)
	if !ok {
		return api.ExecResult{}, fmt.Errorf("resource %q not found", req.Resource)
	}
	execer, ok := e.cfg.Execers[r.Kind]
	if !ok {
		return api.ExecResult{}, fmt.Errorf("exec not supported for kind %q", r.Kind)
	}
	return execer.Exec(ctx, req.Worktree, req.Resource, req.Cmd, as.env)
}
