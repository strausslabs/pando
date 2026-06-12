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

// Not a git worktree: the reconciler must never track or remove this slug.
const sharedSlug = "_shared"

const sharedPortKey = "__pando_shared__"

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

func (e *Engine) sharedReady(name string) bool {
	e.mu.RLock()
	defer e.mu.RUnlock()
	s := e.stacks[sharedSlug]
	if s == nil {
		return false
	}
	return s.phases[name].OK()
}

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
