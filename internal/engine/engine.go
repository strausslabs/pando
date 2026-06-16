package engine

import (
	"context"
	"fmt"
	"sort"
	"sync"
	"time"

	"github.com/strausslabs/pando/internal/api"
	"github.com/strausslabs/pando/internal/dag"
	"github.com/strausslabs/pando/internal/interp"
	"github.com/strausslabs/pando/internal/logbuf"
	"github.com/strausslabs/pando/internal/probe"
	"github.com/strausslabs/pando/internal/resource"
	"github.com/strausslabs/pando/internal/scheduler"
	"github.com/strausslabs/pando/internal/worktree"
)

const configResource = "pando.config"

type Execer interface {
	Exec(ctx context.Context, worktree, resource string, cmd []string, env scheduler.Env) (api.ExecResult, error)
}

type RunStore interface {
	scheduler.RunStore
	Reset(worktree string)
	Forget(worktree, resource string)
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

type Engine struct {
	cfg Config

	mu         sync.RWMutex
	stacks     map[string]*activeStack
	configErrs map[string]configFault
}

type configFault struct {
	branch string
	msg    string
}

type activeStack struct {
	info   api.WorktreeInfo
	stack  *resource.Stack
	graph  *dag.Graph
	sched  *scheduler.Scheduler
	env    scheduler.Env
	phases map[string]scheduler.Phase
	errs   map[string]string

	mu             sync.Mutex
	nextRun        map[string]time.Time
	periodicCancel context.CancelFunc
	live           *liveWatcher
	liveRunning    map[string]*sync.Mutex
}

func New(cfg Config) *Engine {
	if cfg.Clock == nil {
		cfg.Clock = time.Now
	}
	return &Engine{
		cfg:        cfg,
		stacks:     map[string]*activeStack{},
		configErrs: map[string]configFault{},
	}
}

func (e *Engine) ReportConfigError(slug, branch, msg string) {
	e.mu.Lock()
	prev, had := e.configErrs[slug]
	e.configErrs[slug] = configFault{branch: branch, msg: msg}
	e.mu.Unlock()
	// Dedup: the reconciler re-reports on every poll tick; stream only on change.
	if had && prev.msg == msg {
		return
	}
	e.streamConfig(slug, logbuf.Stderr, msg)
	if e.cfg.Logs != nil {
		e.cfg.Logs.PublishPhase(slug, configResource, string(scheduler.PhaseFailed))
	}
}

func (e *Engine) ClearConfigError(slug string) {
	e.mu.Lock()
	_, had := e.configErrs[slug]
	delete(e.configErrs, slug)
	e.mu.Unlock()
	if had {
		e.streamConfig(slug, logbuf.System, "config recovered")
	}
}

func (e *Engine) streamConfig(slug string, stream logbuf.Stream, text string) {
	e.logAppend(slug, configResource, stream, text)
}

func (e *Engine) logAppend(worktree, name string, stream logbuf.Stream, text string) {
	if e.cfg.Logs == nil {
		return
	}
	e.cfg.Logs.Append(worktree, name, stream, text,
		func() logbuf.Line { return logbuf.Line{Time: e.cfg.Clock()} })
}

func (e *Engine) Register(wt worktree.Worktree, stack *resource.Stack) error {
	as, err := e.compile(api.WorktreeInfo{Path: wt.Path, Branch: wt.Branch, Head: wt.Head, Slug: wt.Slug}, stack)
	if err != nil {
		return err
	}
	e.mu.Lock()
	e.stacks[wt.Slug] = as
	e.mu.Unlock()
	return nil
}

// Register and Reload both funnel through compile so shared-resource handling can't drift.
func (e *Engine) compile(info api.WorktreeInfo, stack *resource.Stack) (*activeStack, error) {
	sharedRes, localRes := partitionShared(stack)
	if err := e.mergeShared(sharedRes); err != nil {
		return nil, err
	}
	local := &resource.Stack{Name: stack.Name, Resources: localRes}

	g, err := dag.BuildExternal(local, e.sharedNames())
	if err != nil {
		return nil, err
	}
	ports := e.cfg.Allocator.Allocate(info.Path, resourceNames(local))
	for name, p := range e.sharedPorts() {
		if _, ok := ports[name]; !ok {
			ports[name] = p
		}
	}
	env := scheduler.Env{
		Worktree: info.Slug,
		Project:  worktree.ProjectName(e.cfg.StackName, info.Slug),
		Dir:      info.Path,
		Ports:    ports,
		Vars:     map[string]string{},
	}
	return e.newActiveStack(info.Slug, api.WorktreeInfo{Path: info.Path, Branch: info.Branch, Head: info.Head, Slug: info.Slug, Ports: ports}, local, g, env), nil
}

func (e *Engine) newActiveStack(slug string, info api.WorktreeInfo, stack *resource.Stack, g *dag.Graph, env scheduler.Env) *activeStack {
	as := &activeStack{
		info:    info,
		stack:   stack,
		graph:   g,
		env:     env,
		phases:  map[string]scheduler.Phase{},
		errs:    map[string]string{},
		nextRun: map[string]time.Time{},
	}
	as.sched = e.newScheduler(slug, g, env, as)
	return as
}

func (e *Engine) newScheduler(slug string, g *dag.Graph, env scheduler.Env, as *activeStack) *scheduler.Scheduler {
	opts := scheduler.Options{
		Executors: e.cfg.Executors,
		Store:     e.cfg.Store,
		Env:       env,
		OnState:   e.stateHandler(slug, as),
		WaitReady: e.waitReady,
		InputHash: func(r *resource.Resource) string { return e.inputHash(as.info.Path, r) },
	}
	if slug != sharedSlug {
		opts.ExternalReady = e.sharedReady
	}
	return scheduler.New(g, opts)
}

func resourceNames(stack *resource.Stack) []string {
	names := make([]string, 0, len(stack.Resources))
	for _, r := range stack.Resources {
		names = append(names, r.Name)
	}
	return names
}

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

	newAS, err := e.compile(info, next)
	if err != nil {
		return err
	}
	diff := resource.DiffStacks(old, newAS.stack)

	// No-op early-return required: swapping an unchanged stack orphans in-flight executor goroutines holding the old activeStack, dropping their final phase updates.
	if len(diff.Added) == 0 && len(diff.Changed) == 0 && len(diff.Removed) == 0 {
		return nil
	}

	for _, name := range diff.Removed {
		if r, found := old.Get(name); found {
			if exec, ok := e.cfg.Executors[r.Kind]; ok {
				_ = exec.Stop(ctx, r, as.env)
			}
		}
	}

	changed := markSet(diff.Added, diff.Changed)
	seed := map[string]scheduler.Phase{}
	for _, r := range newAS.stack.Resources {
		if changed[r.Name] {
			continue
		}
		if p, ok := oldPhases[r.Name]; ok {
			seed[r.Name] = p
			newAS.phases[r.Name] = p
		}
	}
	newAS.sched.Seed(seed)

	as.stopPeriodic()
	as.stopWatchers()

	e.mu.Lock()
	e.stacks[slug] = newAS
	e.mu.Unlock()

	e.startPeriodic(newAS)
	e.startWatchers(newAS)

	dirty := append(append([]string{}, diff.Added...), diff.Changed...)
	if len(dirty) == 0 {
		return nil
	}
	return newAS.sched.UpSubset(ctx, dirty...)
}

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
	as.stopPeriodic()
	as.stopWatchers()
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
		switch {
		case ns.Err != nil:
			as.errs[ns.Name] = ns.Err.Error()
		default:
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
	if slug != sharedSlug {
		if err := e.ensureSharedUp(ctx); err != nil {
			return err
		}
	}
	err = as.sched.Up(ctx)
	e.startPeriodic(as)
	e.startWatchers(as)
	return err
}

func (e *Engine) Down(ctx context.Context, slug string) error {
	as, err := e.lookup(slug)
	if err != nil {
		return err
	}
	as.stopPeriodic()
	as.stopWatchers()
	return as.sched.Down(ctx)
}

func (e *Engine) Shutdown(ctx context.Context) {
	e.mu.RLock()
	slugs := make([]string, 0, len(e.stacks))
	for slug := range e.stacks {
		slugs = append(slugs, slug)
	}
	e.mu.RUnlock()
	for _, slug := range slugs {
		_ = e.Down(ctx, slug)
	}
}

func (e *Engine) Restart(ctx context.Context, slug, name string) error {
	as, err := e.lookup(slug)
	if err != nil {
		return err
	}
	if _, ok := as.stack.Get(name); !ok {
		return fmt.Errorf("resource %q not found in worktree %q", name, slug)
	}
	if e.cfg.Store != nil {
		e.cfg.Store.Forget(slug, name)
	}
	return as.sched.UpSubset(ctx, name)
}

func (e *Engine) Rebuild(ctx context.Context, slug, name string) error {
	return e.Restart(ctx, slug, name)
}

func (e *Engine) Trigger(ctx context.Context, slug, name string) error {
	return e.Restart(ctx, slug, name)
}

func (e *Engine) sortedSlugs() []string {
	slugs := make([]string, 0, len(e.stacks))
	for slug := range e.stacks {
		slugs = append(slugs, slug)
	}
	sort.Slice(slugs, func(i, j int) bool {
		bi, bj := e.stacks[slugs[i]].info.Branch, e.stacks[slugs[j]].info.Branch
		if bi != bj {
			return bi < bj
		}
		return slugs[i] < slugs[j]
	})
	return slugs
}

func (e *Engine) ListWorktrees(ctx context.Context) ([]api.WorktreeInfo, error) {
	e.mu.RLock()
	defer e.mu.RUnlock()
	out := make([]api.WorktreeInfo, 0, len(e.stacks))
	for _, slug := range e.sortedSlugs() {
		out = append(out, e.stacks[slug].info)
	}
	return out, nil
}

func (e *Engine) Status(ctx context.Context) ([]api.WorktreeStatus, error) {
	e.mu.RLock()
	defer e.mu.RUnlock()
	out := make([]api.WorktreeStatus, 0, len(e.stacks))
	for _, slug := range e.sortedSlugs() {
		as := e.stacks[slug]
		ws := api.WorktreeStatus{Worktree: slug, Branch: as.info.Branch, Head: as.info.Head}
		if f, ok := e.configErrs[slug]; ok {
			ws.Error = f.msg
		}
		for _, r := range as.stack.Resources {
			phase := as.phases[r.Name]
			rs := api.ResourceStatus{
				Name:  r.Name,
				Kind:  string(r.Kind),
				Phase: string(phase),
				Ready: phase.OK(),
				Port:  as.info.Ports[r.Name],
				Error: as.errs[r.Name],
			}
			if phase.OK() {
				if s, ok := e.cfg.Executors[r.Kind].(scheduler.Sampler); ok {
					if u, ok := s.Sample(ctx, r, as.env); ok {
						rs.MemBytes = u.MemBytes
						rs.CPUPercent = u.CPUPercent
					}
				}
			}
			if r.Compose != nil {
				rs.MemLimitBytes = r.Compose.Memory
			}
			if r.IsPeriodic() {
				rs.EverySeconds = int64(r.Every.Seconds())
				as.mu.Lock()
				t, ok := as.nextRun[r.Name]
				as.mu.Unlock()
				if ok {
					rs.NextRunUnix = t.Unix()
				}
			}
			ws.Resources = append(ws.Resources, rs)
		}
		out = append(out, ws)
	}
	for _, slug := range e.faultedUnregistered() {
		f := e.configErrs[slug]
		out = append(out, api.WorktreeStatus{Worktree: slug, Branch: f.branch, Error: f.msg})
	}
	return out, nil
}

func (e *Engine) faultedUnregistered() []string {
	var slugs []string
	for slug := range e.configErrs {
		if _, registered := e.stacks[slug]; !registered {
			slugs = append(slugs, slug)
		}
	}
	sort.Strings(slugs)
	return slugs
}

func (e *Engine) Logs(ctx context.Context, q api.LogQuery) ([]api.LogLine, error) {
	if e.cfg.Logs == nil {
		return nil, nil
	}
	if q.Resource != "" && q.Resource != configResource {
		as, err := e.lookup(q.Worktree)
		if err != nil {
			return nil, err
		}
		if _, ok := as.stack.Get(q.Resource); !ok {
			return nil, fmt.Errorf("resource %q not found in worktree %q", q.Resource, q.Worktree)
		}
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
