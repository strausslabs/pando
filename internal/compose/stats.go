package compose

import (
	"context"
	"encoding/json"

	"github.com/docker/docker/api/types/container"

	"github.com/guyStrauss/pando/internal/resource"
	"github.com/guyStrauss/pando/internal/scheduler"
)

// Sample reports the live container footprint for a compose resource using the
// Docker one-shot stats endpoint. It mirrors `docker stats` accounting: memory
// excludes the cgroup page cache, and CPU is the share-of-one-core percentage
// derived from the delta between the current and previous CPU snapshots.
//
// One-shot stats carry no prior CPU sample on the daemon side, so CPUPercent
// may legitimately be 0; memory is always meaningful and is the priority. ok is
// false when the container is missing/stopped or the stats body fails to decode.
func (b *Backend) Sample(ctx context.Context, r *resource.Resource, env scheduler.Env) (scheduler.Usage, bool) {
	resp, err := b.cli.ContainerStatsOneShot(ctx, containerName(env.Project, r.Name))
	if err != nil {
		return scheduler.Usage{}, false
	}
	defer resp.Body.Close()

	var s container.StatsResponse
	if err := json.NewDecoder(resp.Body).Decode(&s); err != nil {
		return scheduler.Usage{}, false
	}

	return scheduler.Usage{
		MemBytes:   memBytes(s.MemoryStats),
		CPUPercent: cpuPercent(s.CPUStats, s.PreCPUStats),
	}, true
}

// memBytes returns container memory usage with the cgroup page cache subtracted,
// matching `docker stats`. cgroup v1 reports the reclaimable cache as "cache";
// cgroup v2 reports it as "inactive_file". Subtraction only applies when the
// cache value does not exceed Usage, guarding against underflow.
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

// cpuPercent applies the standard docker formula: the container's CPU delta as a
// fraction of the system CPU delta, scaled across the online cores. Returns 0
// when either delta is non-positive (e.g. the first one-shot sample).
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
