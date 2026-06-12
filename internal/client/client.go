package client

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"strconv"
	"time"

	"github.com/guyStrauss/pando/internal/api"
)

type Client struct {
	http *http.Client
}

func New(socketPath string) *Client {
	return &Client{
		http: &http.Client{
			Transport: &http.Transport{
				DialContext: func(ctx context.Context, _, _ string) (net.Conn, error) {
					var d net.Dialer
					return d.DialContext(ctx, "unix", socketPath)
				},
			},
			Timeout: 0,
		},
	}
}

const base = "http://pando"

func (c *Client) Health(ctx context.Context) error {
	return c.get(ctx, "/healthz", nil)
}

func (c *Client) Status(ctx context.Context) ([]api.WorktreeStatus, error) {
	var out []api.WorktreeStatus
	return out, c.get(ctx, "/status", &out)
}

func (c *Client) ListWorktrees(ctx context.Context) ([]api.WorktreeInfo, error) {
	var out []api.WorktreeInfo
	return out, c.get(ctx, "/worktrees", &out)
}

func (c *Client) Logs(ctx context.Context, q api.LogQuery) ([]api.LogLine, error) {
	v := url.Values{}
	v.Set("worktree", q.Worktree)
	v.Set("resource", q.Resource)
	if q.Tail > 0 {
		v.Set("tail", strconv.Itoa(q.Tail))
	}
	if q.Grep != "" {
		v.Set("grep", q.Grep)
	}
	if !q.Since.IsZero() {
		v.Set("since", time.Since(q.Since).String())
	}
	var out []api.LogLine
	return out, c.get(ctx, "/logs?"+v.Encode(), &out)
}

func (c *Client) Up(ctx context.Context, worktree string, force bool) error {
	return c.post(ctx, "/up", map[string]any{"worktree": worktree, "force": force}, nil)
}

func (c *Client) Down(ctx context.Context, worktree string) error {
	return c.post(ctx, "/down", map[string]any{"worktree": worktree}, nil)
}

func (c *Client) Restart(ctx context.Context, worktree, resource string) error {
	return c.resourceAction(ctx, "/restart", worktree, resource)
}

func (c *Client) Rebuild(ctx context.Context, worktree, resource string) error {
	return c.resourceAction(ctx, "/rebuild", worktree, resource)
}

func (c *Client) Trigger(ctx context.Context, worktree, resource string) error {
	return c.resourceAction(ctx, "/trigger", worktree, resource)
}

func (c *Client) Exec(ctx context.Context, req api.ExecRequest) (api.ExecResult, error) {
	var out api.ExecResult
	return out, c.post(ctx, "/exec", req, &out)
}

func (c *Client) resourceAction(ctx context.Context, path, worktree, resource string) error {
	return c.post(ctx, path, map[string]any{"worktree": worktree, "resource": resource}, nil)
}

func (c *Client) get(ctx context.Context, path string, out any) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, base+path, nil)
	if err != nil {
		return err
	}
	return c.do(req, out)
}

func (c *Client) post(ctx context.Context, path string, body, out any) error {
	var buf bytes.Buffer
	if err := json.NewEncoder(&buf).Encode(body); err != nil {
		return err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, base+path, &buf)
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	return c.do(req, out)
}

func (c *Client) do(req *http.Request, out any) error {
	resp, err := c.http.Do(req)
	if err != nil {
		return fmt.Errorf("daemon unreachable (is `pando daemon` running?): %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 400 {
		var e struct {
			Error string `json:"error"`
		}
		_ = json.NewDecoder(resp.Body).Decode(&e)
		if e.Error != "" {
			return fmt.Errorf("%s", e.Error)
		}
		return fmt.Errorf("daemon returned %d", resp.StatusCode)
	}
	if out == nil {
		return nil
	}
	return json.NewDecoder(resp.Body).Decode(out)
}
