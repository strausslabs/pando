package scheduler

import (
	"context"

	"github.com/guyStrauss/pando/internal/resource"
)

type Phase string

const (
	PhasePending      Phase = "pending"
	PhaseWaiting      Phase = "waiting"
	PhaseStarting     Phase = "starting"
	PhaseHealthy      Phase = "healthy"
	PhaseRunning      Phase = "running"
	PhaseDone         Phase = "done"
	PhaseFailed       Phase = "failed"
	PhaseSkipped      Phase = "skipped"
	PhaseBlocked      Phase = "blocked"
	PhaseStopped      Phase = "stopped"
	PhaseShuttingDown Phase = "shuttingDown"
	PhaseLiveUpdating Phase = "liveUpdating"
)

func (p Phase) Terminal() bool {
	switch p {
	case PhaseDone, PhaseFailed, PhaseSkipped, PhaseBlocked, PhaseStopped:
		return true
	}
	return false
}

func (p Phase) OK() bool {
	switch p {
	case PhaseHealthy, PhaseRunning, PhaseDone, PhaseSkipped:
		return true
	}
	return false
}

// Executor runs a single resource to a settled state. Start blocks until the
// resource is healthy (long-running) or has exited (tasks), or the context is
// cancelled. Implementations report progress through the provided Reporter.
type Executor interface {
	Start(ctx context.Context, r *resource.Resource, env Env, rep Reporter) error
	Stop(ctx context.Context, r *resource.Resource, env Env) error
}

// Usage is a single resource footprint sample. CPUPercent is a share of one
// core, so it may exceed 100 for multi-threaded work.
type Usage struct {
	MemBytes   uint64
	CPUPercent float64
}

// Sampler is the optional capability an executor implements to report a running
// resource's footprint. The daemon type-asserts for it; ok is false when the
// resource is not running or no sample is available.
type Sampler interface {
	Sample(ctx context.Context, r *resource.Resource, env Env) (Usage, bool)
}

// Env carries per-worktree resolved values (ports, project name, vars) handed
// to executors at run time.
type Env struct {
	Worktree string
	Project  string
	Ports    map[string]int
	Vars     map[string]string
}

type Reporter interface {
	Phase(phase Phase)
	Logf(format string, args ...any)
}
