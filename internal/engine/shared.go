package engine

import (
	"context"
	"fmt"
	"time"

	"github.com/guyStrauss/pando/internal/api"
	"github.com/guyStrauss/pando/internal/dag"
	"github.com/guyStrauss/pando/internal/resource"
	"github.com/guyStrauss/pando/internal/scheduler"
)

// sharedSlug is the reserved worktree slug under which daemon-level shared
// (singleton) resources live. It is not a git worktree, so the reconciler never
// tracks or removes it, but Status/Down/Shutdown/periodic all treat it like any
// other stack.
const sharedSlug = "_shared"

// sharedPortKey is the fixed allocator key for shared resources, so their ports
// are stable and independent of any worktree path.
const sharedPortKey = "__pando_shared__"

// partitionShared splits a worktree's resources into the shared singletons and
// the rest.
func partitionShared(stack *resource.Stack) (shared, local []*resource.Resource) {
	for _, r := range stack.Resources {
		if r.Shared {
			shared = append(shared, r)
			continue
		}
		local = append(local, r)
	}
	return shared, local
}

// mergeShared folds a worktree's shared resources into the daemon-level shared
// stack (first definition of a name wins) and (re)builds its scheduler. Safe to
// call repeatedly as worktrees register; a no-op when nothing new appears.
func (e *Engine) mergeShared(incoming []*resource.Resource) error {
	if len(incoming) == 0 {
		return nil
	}
	e.mu.Lock()
	cur := e.stacks[sharedSlug]
	existing := map[string]bool{}
	var resources []*resource.Resource
	if cur != nil {
		for _, r := range cur.stack.Resources {
			existing[r.Name] = true
			resources = append(resources, r)
		}
	}
	added := false
	for _, r := range incoming {
		if !existing[r.Name] {
			existing[r.Name] = true
			resources = append(resources, r)
			added = true
		}
	}
	if !added && cur != nil {
		e.mu.Unlock()
		return nil
	}
	e.mu.Unlock()

	// A shared resource is daemon-level, so it cannot reach a per-worktree
	// resource: every dependency it declares must itself be shared.
	shared := make(map[string]bool, len(resources))
	for _, r := range resources {
		shared[r.Name] = true
	}
	for _, r := range resources {
		for _, d := range r.AllDeps() {
			if !shared[d] {
				return fmt.Errorf("shared resource %q may only depend on shared resources, but depends on %q", r.Name, d)
			}
		}
	}

	stack := &resource.Stack{Name: e.cfg.StackName + "-shared", Resources: resources}
	g, err := dag.Build(stack)
	if err != nil {
		return err
	}
	ports := e.cfg.Allocator.Allocate(sharedPortKey, resourceNames(stack))
	env := scheduler.Env{
		Worktree: sharedSlug,
		Project:  e.cfg.StackName + "-shared",
		Ports:    ports,
		Vars:     map[string]string{},
	}

	as := &activeStack{
		info:    api.WorktreeInfo{Branch: "shared", Slug: sharedSlug, Ports: ports},
		stack:   stack,
		graph:   g,
		env:     env,
		phases:  map[string]scheduler.Phase{},
		errs:    map[string]string{},
		nextRun: map[string]time.Time{},
	}
	as.sched = e.newScheduler(sharedSlug, g, env, as)

	e.mu.Lock()
	// Carry forward phases for already-running shared resources so a new
	// worktree registering does not restart them.
	if cur != nil {
		for name, p := range cur.phases {
			as.phases[name] = p
		}
		as.sched.Seed(cur.phases)
	}
	e.stacks[sharedSlug] = as
	e.mu.Unlock()
	return nil
}

// sharedNames returns the set of shared resource names currently registered.
func (e *Engine) sharedNames() map[string]bool {
	e.mu.RLock()
	defer e.mu.RUnlock()
	out := map[string]bool{}
	if s := e.stacks[sharedSlug]; s != nil {
		for _, r := range s.stack.Resources {
			out[r.Name] = true
		}
	}
	return out
}

// sharedPorts returns the shared resources' allocated ports, merged into each
// worktree's env so a resource depending on a shared one can reach it via
// $PORT_<name>.
func (e *Engine) sharedPorts() map[string]int {
	e.mu.RLock()
	defer e.mu.RUnlock()
	out := map[string]int{}
	if s := e.stacks[sharedSlug]; s != nil {
		for name, p := range s.info.Ports {
			out[name] = p
		}
	}
	return out
}

// sharedReady reports whether a shared resource has reached an OK phase. Used as
// the worktree schedulers' ExternalReady gate.
func (e *Engine) sharedReady(name string) bool {
	e.mu.RLock()
	defer e.mu.RUnlock()
	s := e.stacks[sharedSlug]
	if s == nil {
		return false
	}
	return s.phases[name].OK()
}

// ensureSharedUp brings the shared stack to a settled state once, before a
// worktree that depends on it comes up. A no-op when there are no shared
// resources.
func (e *Engine) ensureSharedUp(ctx context.Context) error {
	e.mu.RLock()
	s := e.stacks[sharedSlug]
	e.mu.RUnlock()
	if s == nil {
		return nil
	}
	err := s.sched.Up(ctx)
	e.startPeriodic(s)
	return err
}
