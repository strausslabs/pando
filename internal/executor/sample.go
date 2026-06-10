package executor

import (
	"context"

	"github.com/shirou/gopsutil/v3/process"

	"github.com/guyStrauss/pando/internal/resource"
	"github.com/guyStrauss/pando/internal/scheduler"
)

// Sample reports the live resource footprint of a managed host process. Because
// resources launch via `sh -c`, the actual workload is usually a child of the
// shell, so we sum RSS and CPU across the root process and all its descendants.
// It is called on a poll loop and must not block: gopsutil's CPUPercent may
// return 0 on the first observation, which is acceptable.
func (e *Engine) Sample(ctx context.Context, r *resource.Resource, env scheduler.Env) (scheduler.Usage, bool) {
	e.mu.Lock()
	m, ok := e.running[key(env.Worktree, r.Name)]
	e.mu.Unlock()
	if !ok || m.cmd.Process == nil {
		return scheduler.Usage{}, false
	}

	root, err := process.NewProcess(int32(m.cmd.Process.Pid))
	if err != nil {
		// Root process is no longer in the OS table.
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

// collectTree returns root and all of its descendants. Errors fetching a node's
// children are skipped so a single transient failure (e.g. a child exiting
// mid-walk) does not abort the whole sample.
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
