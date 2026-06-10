package api

import (
	"context"
	"time"
)

// StackOps is the single set of operations every face (CLI, web UI, MCP) calls.
// Defining it once keeps the daemon, clients, and agent adapter in lockstep:
// add a backend and every face gets it; add a face and it speaks this contract.
type StackOps interface {
	Status(ctx context.Context) ([]WorktreeStatus, error)
	Logs(ctx context.Context, q LogQuery) ([]LogLine, error)
	Exec(ctx context.Context, req ExecRequest) (ExecResult, error)
	Up(ctx context.Context, worktree string, force bool) error
	Down(ctx context.Context, worktree string) error
	Restart(ctx context.Context, worktree, resource string) error
	Rebuild(ctx context.Context, worktree, resource string) error
	Trigger(ctx context.Context, worktree, resource string) error
	ListWorktrees(ctx context.Context) ([]WorktreeInfo, error)
}

type WorktreeInfo struct {
	Path   string         `json:"path"`
	Branch string         `json:"branch"`
	Head   string         `json:"head"`
	Slug   string         `json:"slug"`
	Ports  map[string]int `json:"ports"`
}

type ResourceStatus struct {
	Name  string `json:"name"`
	Kind  string `json:"kind"`
	Phase string `json:"phase"`
	Ready bool   `json:"ready"`
	Port  int    `json:"port,omitempty"`
	Error string `json:"error,omitempty"`

	// Live footprint, zero when not measured. MemBytes is RSS for host processes,
	// container memory for compose resources.
	MemBytes   uint64  `json:"memBytes,omitempty"`
	CPUPercent float64 `json:"cpuPercent,omitempty"`

	// Periodic schedule, zero when the resource is not periodic.
	EverySeconds int64 `json:"everySeconds,omitempty"`
	NextRunUnix  int64 `json:"nextRunUnix,omitempty"`

	// Preview is true for web resources the dashboard renders as a live iframe.
	Preview bool `json:"preview,omitempty"`
}

type WorktreeStatus struct {
	Worktree  string           `json:"worktree"`
	Branch    string           `json:"branch"`
	Head      string           `json:"head"`
	Resources []ResourceStatus `json:"resources"`
}

type LogQuery struct {
	Worktree string    `json:"worktree"`
	Resource string    `json:"resource"`
	Tail     int       `json:"tail,omitempty"`
	Since    time.Time `json:"since,omitempty"`
	Grep     string    `json:"grep,omitempty"`
}

type LogLine struct {
	Seq      uint64    `json:"seq"`
	Time     time.Time `json:"time"`
	Worktree string    `json:"worktree"`
	Resource string    `json:"resource"`
	Stream   string    `json:"stream"`
	Text     string    `json:"text"`
}

type ExecRequest struct {
	Worktree string   `json:"worktree"`
	Resource string   `json:"resource"`
	Cmd      []string `json:"cmd"`
}

type ExecResult struct {
	Stdout   string `json:"stdout"`
	Stderr   string `json:"stderr"`
	ExitCode int    `json:"exitCode"`
}
