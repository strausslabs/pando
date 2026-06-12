package executor

import (
	"context"

	"github.com/shirou/gopsutil/v3/process"

	"github.com/guyStrauss/pando/internal/resource"
	"github.com/guyStrauss/pando/internal/scheduler"
)

func (e *Engine) Sample(ctx context.Context, r *resource.Resource, env scheduler.Env) (scheduler.Usage, bool) {
	e.mu.Lock()
	m, ok := e.running[key(env.Worktree, r.Name)]
	e.mu.Unlock()
	if !ok || m.cmd.Process == nil {
		return scheduler.Usage{}, false
	}

	root, err := process.NewProcess(int32(m.cmd.Process.Pid))
	if err != nil {
		return scheduler.Usage{}, false
	}

	var usage scheduler.Usage
	procs := collectTree(root)
	for _, p := range procs {
		if mi, err := p.MemoryInfo(); err == nil && mi != nil {
			usage.MemBytes += mi.RSS
		}
		if pct, err := p.CPUPercent(); err == nil {
			usage.CPUPercent += pct
		}
	}
	return usage, true
}

func (e *Engine) Sync(ctx context.Context, r *resource.Resource, env scheduler.Env, localPath, containerPath string) error {
	return nil
}

func collectTree(root *process.Process) []*process.Process {
	out := []*process.Process{root}
	queue := []*process.Process{root}
	for len(queue) > 0 {
		p := queue[0]
		queue = queue[1:]
		children, err := p.Children()
		if err != nil {
			continue
		}
		for _, c := range children {
			out = append(out, c)
			queue = append(queue, c)
		}
	}
	return out
}
