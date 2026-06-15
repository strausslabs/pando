package compose

import (
	"context"
	"encoding/json"

	"github.com/docker/docker/api/types/container"

	"github.com/strausslabs/pando/internal/resource"
	"github.com/strausslabs/pando/internal/scheduler"
)

func (b *Backend) Sample(ctx context.Context, r *resource.Resource, env scheduler.Env) (scheduler.Usage, bool) {
	resp, err := b.cli.ContainerStatsOneShot(ctx, containerName(env.Project, r.Name))
	if err != nil {
		return scheduler.Usage{}, false
	}
	defer func() { _ = resp.Body.Close() }()

	var s container.StatsResponse
	if err := json.NewDecoder(resp.Body).Decode(&s); err != nil {
		return scheduler.Usage{}, false
	}

	return scheduler.Usage{
		MemBytes:   memBytes(s.MemoryStats),
		CPUPercent: cpuPercent(s.CPUStats, s.PreCPUStats),
	}, true
}

// docker-stats accounting: subtract reclaimable page cache (cgroup v1 "cache",
// v2 "inactive_file") from usage; the cache<=usage guard prevents underflow.
func memBytes(m container.MemoryStats) uint64 {
	usage := m.Usage
	cache := m.Stats["cache"]
	if cache == 0 {
		cache = m.Stats["inactive_file"]
	}
	if cache <= usage {
		return usage - cache
	}
	return usage
}

func cpuPercent(cur, pre container.CPUStats) float64 {
	cpuDelta := float64(cur.CPUUsage.TotalUsage) - float64(pre.CPUUsage.TotalUsage)
	sysDelta := float64(cur.SystemUsage) - float64(pre.SystemUsage)
	if sysDelta <= 0 || cpuDelta <= 0 {
		return 0
	}
	onlineCPUs := float64(cur.OnlineCPUs)
	if onlineCPUs == 0 {
		if n := len(cur.CPUUsage.PercpuUsage); n > 0 {
			onlineCPUs = float64(n)
		} else {
			onlineCPUs = 1
		}
	}
	return (cpuDelta / sysDelta) * onlineCPUs * 100
}
