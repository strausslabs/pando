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

type Executor interface {
	Start(ctx context.Context, r *resource.Resource, env Env, rep Reporter) error
	Stop(ctx context.Context, r *resource.Resource, env Env) error
}

// CPUPercent is a share of one core, so it may exceed 100 for multi-threaded work.
type Usage struct {
	MemBytes   uint64
	CPUPercent float64
}

type Sampler interface {
	Sample(ctx context.Context, r *resource.Resource, env Env) (Usage, bool)
}

type Syncer interface {
	Sync(ctx context.Context, r *resource.Resource, env Env, localPath, containerPath string) error
}

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
