package compose

import (
	"testing"

	"github.com/docker/docker/api/types/container"
)

func TestMemBytes(t *testing.T) {
	tests := []struct {
		name string
		mem  container.MemoryStats
		want uint64
	}{
		{"cgroup v1 cache", container.MemoryStats{Usage: 100, Stats: map[string]uint64{"cache": 30}}, 70},
		{"cgroup v2 inactive_file", container.MemoryStats{Usage: 100, Stats: map[string]uint64{"inactive_file": 40}}, 60},
		{"cache exceeds usage clamps", container.MemoryStats{Usage: 50, Stats: map[string]uint64{"cache": 80}}, 50},
		{"no cache passthrough", container.MemoryStats{Usage: 100}, 100},
		{"cache wins over inactive_file", container.MemoryStats{Usage: 100, Stats: map[string]uint64{"cache": 10, "inactive_file": 90}}, 90},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := memBytes(tt.mem); got != tt.want {
				t.Errorf("memBytes = %d, want %d", got, tt.want)
			}
		})
	}
}

func TestCPUPercent(t *testing.T) {
	cpu := func(total, system uint64, online uint32, percpu int) container.CPUStats {
		s := container.CPUStats{SystemUsage: system, OnlineCPUs: online}
		s.CPUUsage.TotalUsage = total
		if percpu > 0 {
			s.CPUUsage.PercpuUsage = make([]uint64, percpu)
		}
		return s
	}
	tests := []struct {
		name     string
		cur, pre container.CPUStats
		want     float64
	}{
		{"two cpus", cpu(200, 2000, 2, 0), cpu(100, 1000, 2, 0), 20},
		{"sys delta zero", cpu(200, 1000, 2, 0), cpu(100, 1000, 2, 0), 0},
		{"cpu delta zero", cpu(100, 2000, 2, 0), cpu(100, 1000, 2, 0), 0},
		{"online from percpu", cpu(200, 2000, 0, 4), cpu(100, 1000, 0, 0), 40},
		{"online defaults to one", cpu(200, 2000, 0, 0), cpu(100, 1000, 0, 0), 10},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := cpuPercent(tt.cur, tt.pre); got != tt.want {
				t.Errorf("cpuPercent = %v, want %v", got, tt.want)
			}
		})
	}
}
