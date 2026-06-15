package mcpserver

import (
	"context"
	"fmt"
	"regexp"
	"strings"

	"github.com/guyStrauss/pando/internal/api"
	"github.com/guyStrauss/pando/internal/client"
	"github.com/guyStrauss/pando/internal/discovery"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

type Deps struct {
	Resolve func(ctx context.Context) (discovery.Info, bool, bool)
	Dial    func(socket string) Daemon
}

type Daemon interface {
	Status(ctx context.Context) ([]api.WorktreeStatus, error)
	Version(ctx context.Context) (api.UpdateStatus, error)
	ListWorktrees(ctx context.Context) ([]api.WorktreeInfo, error)
	Logs(ctx context.Context, q api.LogQuery) ([]api.LogLine, error)
	Up(ctx context.Context, worktree string, force bool) error
	Down(ctx context.Context, worktree string) error
	Restart(ctx context.Context, worktree, resource string) error
	Exec(ctx context.Context, req api.ExecRequest) (api.ExecResult, error)
}

func defaultDeps() Deps {
	return Deps{
		Resolve: discovery.Resolve,
		Dial:    func(socket string) Daemon { return client.New(socket) },
	}
}

func NewServer(version string, deps *Deps) *mcp.Server {
	d := defaultDeps()
	if deps != nil {
		d = *deps
	}
	s := mcp.NewServer(&mcp.Implementation{
		Name:    "pando",
		Title:   "Pando dev environments",
		Version: version,
	}, nil)
	register(s, d)
	return s
}

func register(s *mcp.Server, d Deps) {
	mcp.AddTool(s, &mcp.Tool{
		Name:        "pando_running",
		Description: "Report whether a Pando daemon is managing the current repo. Call this first; if not running, tell the user to run `pando start`.",
	}, runningTool(d))

	mcp.AddTool(s, &mcp.Tool{
		Name:        "pando_status",
		Description: "List every worktree and the phase, port, CPU/memory and any error of each of its resources.",
	}, statusTool(d))

	mcp.AddTool(s, &mcp.Tool{
		Name:        "pando_logs",
		Description: "Fetch recent log lines for one resource in a worktree. Omit worktree to use the only/current one.",
	}, logsTool(d))

	mcp.AddTool(s, &mcp.Tool{
		Name:        "pando_logs_search",
		Description: "Search a resource's logs by regular expression, returning at most the last `tail` matching lines. Use this to find errors or specific output across a long log.",
	}, logsSearchTool(d))

	mcp.AddTool(s, &mcp.Tool{
		Name:        "pando_exec",
		Description: "Run a command inside a running resource (host process or container) and return stdout, stderr and exit code.",
	}, execTool(d))

	mcp.AddTool(s, &mcp.Tool{
		Name:        "pando_up",
		Description: "Bring a worktree's stack up. force re-runs run-once tasks.",
	}, upTool(d))

	mcp.AddTool(s, &mcp.Tool{
		Name:        "pando_down",
		Description: "Tear a worktree's stack down.",
	}, downTool(d))

	mcp.AddTool(s, &mcp.Tool{
		Name:        "pando_restart",
		Description: "Restart a single resource and its dependents in a worktree.",
	}, restartTool(d))
}

func (d Deps) connect(ctx context.Context) (Daemon, *mcp.CallToolResult) {
	info, found, running := d.Resolve(ctx)
	switch {
	case !found:
		return nil, errResult("no Pando daemon for this repo. Run `pando start` in the repo root.")
	case !running:
		return nil, errResult("a Pando daemon was recorded but its socket is unreachable (stale). Run `pando start` again.")
	}
	return d.Dial(info.Socket), nil
}

func errResult(msg string) *mcp.CallToolResult {
	return &mcp.CallToolResult{IsError: true, Content: []mcp.Content{&mcp.TextContent{Text: msg}}}
}

type RunningOut struct {
	Running      bool   `json:"running"`
	Socket       string `json:"socket,omitempty"`
	UIAddr       string `json:"uiAddr,omitempty"`
	GitCommonDir string `json:"gitCommonDir,omitempty"`
	PID          int    `json:"pid,omitempty"`
	Message      string `json:"message"`
}

func runningTool(d Deps) mcp.ToolHandlerFor[struct{}, RunningOut] {
	return func(ctx context.Context, _ *mcp.CallToolRequest, _ struct{}) (*mcp.CallToolResult, RunningOut, error) {
		info, found, running := d.Resolve(ctx)
		switch {
		case !found:
			return nil, RunningOut{Running: false, Message: "no Pando daemon for this repo; run `pando start`"}, nil
		case !running:
			return nil, RunningOut{Running: false, Socket: info.Socket, Message: "daemon recorded but socket is stale; run `pando start`"}, nil
		}
		return nil, RunningOut{
			Running: true, Socket: info.Socket, UIAddr: info.UIAddr,
			GitCommonDir: info.GitCommonDir, PID: info.PID,
			Message: "pando is running for this repo",
		}, nil
	}
}

type StatusOut struct {
	Worktrees       []api.WorktreeStatus `json:"worktrees"`
	Version         string               `json:"version,omitempty"`
	LatestVersion   string               `json:"latestVersion,omitempty"`
	UpdateAvailable bool                 `json:"updateAvailable,omitempty"`
}

func statusTool(d Deps) mcp.ToolHandlerFor[struct{}, StatusOut] {
	return func(ctx context.Context, _ *mcp.CallToolRequest, _ struct{}) (*mcp.CallToolResult, StatusOut, error) {
		cl, errRes := d.connect(ctx)
		if errRes != nil {
			return errRes, StatusOut{}, nil
		}
		st, err := cl.Status(ctx)
		if err != nil {
			return errResult(err.Error()), StatusOut{}, nil
		}
		out := StatusOut{Worktrees: st}
		if up, err := cl.Version(ctx); err == nil {
			out.Version, out.LatestVersion, out.UpdateAvailable = up.Current, up.Latest, up.Available
		}
		return nil, out, nil
	}
}

type LogsIn struct {
	Resource string `json:"resource" jsonschema:"the resource name to read logs for"`
	Worktree string `json:"worktree,omitempty" jsonschema:"worktree slug; omit to use the only or current worktree"`
	Tail     int    `json:"tail,omitempty" jsonschema:"number of trailing lines (default 200)"`
	Grep     string `json:"grep,omitempty" jsonschema:"only lines matching this substring"`
}

type LogsOut struct {
	Worktree string        `json:"worktree"`
	Resource string        `json:"resource"`
	Lines    []api.LogLine `json:"lines"`
}

func logsTool(d Deps) mcp.ToolHandlerFor[LogsIn, LogsOut] {
	return func(ctx context.Context, _ *mcp.CallToolRequest, in LogsIn) (*mcp.CallToolResult, LogsOut, error) {
		cl, errRes := d.connect(ctx)
		if errRes != nil {
			return errRes, LogsOut{}, nil
		}
		wt, errRes := resolveWorktree(ctx, cl, in.Worktree)
		if errRes != nil {
			return errRes, LogsOut{}, nil
		}
		tail := in.Tail
		if tail == 0 {
			tail = 200
		}
		lines, err := cl.Logs(ctx, api.LogQuery{Worktree: wt, Resource: in.Resource, Tail: tail, Grep: in.Grep})
		if err != nil {
			return errResult(err.Error()), LogsOut{}, nil
		}
		return nil, LogsOut{Worktree: wt, Resource: in.Resource, Lines: lines}, nil
	}
}

type LogsSearchIn struct {
	Resource string `json:"resource" jsonschema:"the resource name to search logs for"`
	Pattern  string `json:"pattern" jsonschema:"RE2 regular expression matched against each log line"`
	Worktree string `json:"worktree,omitempty" jsonschema:"worktree slug; omit to use the only or current worktree"`
	Tail     int    `json:"tail,omitempty" jsonschema:"return at most the last N matching lines (default 100)"`
}

type LogsSearchOut struct {
	Worktree string        `json:"worktree"`
	Resource string        `json:"resource"`
	Pattern  string        `json:"pattern"`
	Matched  int           `json:"matched"`
	Lines    []api.LogLine `json:"lines"`
}

func logsSearchTool(d Deps) mcp.ToolHandlerFor[LogsSearchIn, LogsSearchOut] {
	return func(ctx context.Context, _ *mcp.CallToolRequest, in LogsSearchIn) (*mcp.CallToolResult, LogsSearchOut, error) {
		re, err := regexp.Compile(in.Pattern)
		if err != nil {
			return errResult(fmt.Sprintf("invalid regex %q: %v", in.Pattern, err)), LogsSearchOut{}, nil
		}
		cl, errRes := d.connect(ctx)
		if errRes != nil {
			return errRes, LogsSearchOut{}, nil
		}
		wt, errRes := resolveWorktree(ctx, cl, in.Worktree)
		if errRes != nil {
			return errRes, LogsSearchOut{}, nil
		}
		// Pull the whole retained buffer, then filter by regex here so the pattern
		// is full RE2, not the daemon's substring grep.
		lines, err := cl.Logs(ctx, api.LogQuery{Worktree: wt, Resource: in.Resource})
		if err != nil {
			return errResult(err.Error()), LogsSearchOut{}, nil
		}
		matched := lines[:0]
		for _, l := range lines {
			if re.MatchString(l.Text) {
				matched = append(matched, l)
			}
		}
		tail := in.Tail
		if tail == 0 {
			tail = 100
		}
		total := len(matched)
		if total > tail {
			matched = matched[total-tail:]
		}
		return nil, LogsSearchOut{Worktree: wt, Resource: in.Resource, Pattern: in.Pattern, Matched: total, Lines: matched}, nil
	}
}

type ExecIn struct {
	Resource string   `json:"resource" jsonschema:"the resource to run the command inside"`
	Cmd      []string `json:"cmd" jsonschema:"command and arguments, e.g. [\"ls\",\"-la\"]"`
	Worktree string   `json:"worktree,omitempty" jsonschema:"worktree slug; omit to use the only or current worktree"`
}

func execTool(d Deps) mcp.ToolHandlerFor[ExecIn, api.ExecResult] {
	return func(ctx context.Context, _ *mcp.CallToolRequest, in ExecIn) (*mcp.CallToolResult, api.ExecResult, error) {
		cl, errRes := d.connect(ctx)
		if errRes != nil {
			return errRes, api.ExecResult{}, nil
		}
		if len(in.Cmd) == 0 {
			return errResult("cmd must not be empty"), api.ExecResult{}, nil
		}
		wt, errRes := resolveWorktree(ctx, cl, in.Worktree)
		if errRes != nil {
			return errRes, api.ExecResult{}, nil
		}
		res, err := cl.Exec(ctx, api.ExecRequest{Worktree: wt, Resource: in.Resource, Cmd: in.Cmd})
		if err != nil {
			return errResult(err.Error()), api.ExecResult{}, nil
		}
		return nil, res, nil
	}
}

type WorktreeIn struct {
	Worktree string `json:"worktree,omitempty" jsonschema:"worktree slug; omit to use the only or current worktree"`
	Force    bool   `json:"force,omitempty" jsonschema:"re-run run-once tasks"`
}

type OKOut struct {
	OK       bool   `json:"ok"`
	Worktree string `json:"worktree"`
}

func upTool(d Deps) mcp.ToolHandlerFor[WorktreeIn, OKOut] {
	return func(ctx context.Context, _ *mcp.CallToolRequest, in WorktreeIn) (*mcp.CallToolResult, OKOut, error) {
		cl, errRes := d.connect(ctx)
		if errRes != nil {
			return errRes, OKOut{}, nil
		}
		wt, errRes := resolveWorktree(ctx, cl, in.Worktree)
		if errRes != nil {
			return errRes, OKOut{}, nil
		}
		if err := cl.Up(ctx, wt, in.Force); err != nil {
			return errResult(err.Error()), OKOut{}, nil
		}
		return nil, OKOut{OK: true, Worktree: wt}, nil
	}
}

func downTool(d Deps) mcp.ToolHandlerFor[WorktreeIn, OKOut] {
	return func(ctx context.Context, _ *mcp.CallToolRequest, in WorktreeIn) (*mcp.CallToolResult, OKOut, error) {
		cl, errRes := d.connect(ctx)
		if errRes != nil {
			return errRes, OKOut{}, nil
		}
		wt, errRes := resolveWorktree(ctx, cl, in.Worktree)
		if errRes != nil {
			return errRes, OKOut{}, nil
		}
		if err := cl.Down(ctx, wt); err != nil {
			return errResult(err.Error()), OKOut{}, nil
		}
		return nil, OKOut{OK: true, Worktree: wt}, nil
	}
}

type RestartIn struct {
	Resource string `json:"resource" jsonschema:"the resource to restart"`
	Worktree string `json:"worktree,omitempty" jsonschema:"worktree slug; omit to use the only or current worktree"`
}

func restartTool(d Deps) mcp.ToolHandlerFor[RestartIn, OKOut] {
	return func(ctx context.Context, _ *mcp.CallToolRequest, in RestartIn) (*mcp.CallToolResult, OKOut, error) {
		cl, errRes := d.connect(ctx)
		if errRes != nil {
			return errRes, OKOut{}, nil
		}
		wt, errRes := resolveWorktree(ctx, cl, in.Worktree)
		if errRes != nil {
			return errRes, OKOut{}, nil
		}
		if err := cl.Restart(ctx, wt, in.Resource); err != nil {
			return errResult(err.Error()), OKOut{}, nil
		}
		return nil, OKOut{OK: true, Worktree: wt}, nil
	}
}

func resolveWorktree(ctx context.Context, cl Daemon, slug string) (string, *mcp.CallToolResult) {
	if slug != "" {
		return slug, nil
	}
	wts, err := cl.ListWorktrees(ctx)
	if err != nil {
		return "", errResult(err.Error())
	}
	if len(wts) == 1 {
		return wts[0].Slug, nil
	}
	if len(wts) == 0 {
		return "", errResult("no worktrees are registered yet")
	}
	names := make([]string, len(wts))
	for i, w := range wts {
		names[i] = w.Slug
	}
	return "", errResult(fmt.Sprintf("multiple worktrees; pass worktree (one of: %s)", strings.Join(names, ", ")))
}
