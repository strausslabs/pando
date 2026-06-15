package client

import (
	"context"
	"encoding/json"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/strausslabs/pando/internal/api"
)

// newTestClient serves mux over a real unix socket so client.New's transport
// (the unix DialContext) is exercised, not bypassed.
func newTestClient(t *testing.T, mux http.Handler) *Client {
	t.Helper()
	// macOS caps unix socket paths at 104 bytes; t.TempDir() embeds the (long)
	// test name, so use a short top-level temp dir instead.
	dir, err := os.MkdirTemp("", "pc")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(dir) })
	sock := filepath.Join(dir, "s")
	ln, err := net.Listen("unix", sock)
	if err != nil {
		t.Fatal(err)
	}
	srv := &http.Server{Handler: mux}
	go func() { _ = srv.Serve(ln) }()
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), time.Second)
		defer cancel()
		_ = srv.Shutdown(ctx)
	})
	return New(sock)
}

func ctx() context.Context { return context.Background() }

func TestHealth(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /healthz", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})
	if err := newTestClient(t, mux).Health(ctx()); err != nil {
		t.Errorf("Health: %v", err)
	}
}

func TestHealthError(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /healthz", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusServiceUnavailable)
	})
	if err := newTestClient(t, mux).Health(ctx()); err == nil {
		t.Error("503 should error")
	}
}

func TestStatusDecodes(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /status", func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode([]api.WorktreeStatus{{Worktree: "main", Branch: "main"}})
	})
	got, err := newTestClient(t, mux).Status(ctx())
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 || got[0].Worktree != "main" {
		t.Errorf("Status = %+v", got)
	}
}

func TestVersionDecodes(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /version", func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(api.UpdateStatus{Current: "v1", Latest: "v2", Available: true})
	})
	got, err := newTestClient(t, mux).Version(ctx())
	if err != nil {
		t.Fatal(err)
	}
	if got.Current != "v1" || got.Latest != "v2" || !got.Available {
		t.Errorf("Version = %+v", got)
	}
}

func TestListWorktreesDecodes(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /worktrees", func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode([]api.WorktreeInfo{{Slug: "main"}, {Slug: "feat"}})
	})
	got, err := newTestClient(t, mux).ListWorktrees(ctx())
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 || got[1].Slug != "feat" {
		t.Errorf("ListWorktrees = %+v", got)
	}
}

func TestLogsBuildsFullQuery(t *testing.T) {
	var gotQuery string
	mux := http.NewServeMux()
	mux.HandleFunc("GET /logs", func(w http.ResponseWriter, r *http.Request) {
		gotQuery = r.URL.RawQuery
		_ = json.NewEncoder(w).Encode([]api.LogLine{{Text: "x"}})
	})
	q := api.LogQuery{Worktree: "main", Resource: "api", Tail: 50, Grep: "err", Since: time.Now().Add(-time.Minute)}
	if _, err := newTestClient(t, mux).Logs(ctx(), q); err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"worktree=main", "resource=api", "tail=50", "grep=err", "since="} {
		if !contains(gotQuery, want) {
			t.Errorf("query %q missing %q", gotQuery, want)
		}
	}
}

func TestLogsOmitsEmptyParams(t *testing.T) {
	var gotQuery string
	mux := http.NewServeMux()
	mux.HandleFunc("GET /logs", func(w http.ResponseWriter, r *http.Request) {
		gotQuery = r.URL.RawQuery
		_ = json.NewEncoder(w).Encode([]api.LogLine{})
	})
	if _, err := newTestClient(t, mux).Logs(ctx(), api.LogQuery{Worktree: "main", Resource: "api"}); err != nil {
		t.Fatal(err)
	}
	for _, omit := range []string{"tail=", "grep=", "since="} {
		if contains(gotQuery, omit) {
			t.Errorf("query %q should omit %q", gotQuery, omit)
		}
	}
}

func TestUpPostsForce(t *testing.T) {
	var body upBody
	mux := http.NewServeMux()
	mux.HandleFunc("POST /up", func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewDecoder(r.Body).Decode(&body)
		w.WriteHeader(http.StatusOK)
	})
	if err := newTestClient(t, mux).Up(ctx(), "feat", true); err != nil {
		t.Fatal(err)
	}
	if body.Worktree != "feat" || !body.Force {
		t.Errorf("up body = %+v", body)
	}
}

func TestDownPostsWorktree(t *testing.T) {
	var body upBody
	mux := http.NewServeMux()
	mux.HandleFunc("POST /down", func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewDecoder(r.Body).Decode(&body)
		w.WriteHeader(http.StatusOK)
	})
	if err := newTestClient(t, mux).Down(ctx(), "feat"); err != nil {
		t.Fatal(err)
	}
	if body.Worktree != "feat" {
		t.Errorf("down body = %+v", body)
	}
}

func TestResourceActionsRoute(t *testing.T) {
	cases := []struct {
		path string
		call func(*Client) error
	}{
		{"/restart", func(c *Client) error { return c.Restart(ctx(), "w", "api") }},
		{"/rebuild", func(c *Client) error { return c.Rebuild(ctx(), "w", "api") }},
		{"/trigger", func(c *Client) error { return c.Trigger(ctx(), "w", "api") }},
	}
	for _, tc := range cases {
		t.Run(tc.path, func(t *testing.T) {
			var body resourceBody
			mux := http.NewServeMux()
			mux.HandleFunc("POST "+tc.path, func(w http.ResponseWriter, r *http.Request) {
				_ = json.NewDecoder(r.Body).Decode(&body)
				w.WriteHeader(http.StatusOK)
			})
			if err := tc.call(newTestClient(t, mux)); err != nil {
				t.Fatal(err)
			}
			if body.Worktree != "w" || body.Resource != "api" {
				t.Errorf("%s body = %+v", tc.path, body)
			}
		})
	}
}

func TestExecRoundTrip(t *testing.T) {
	var req api.ExecRequest
	mux := http.NewServeMux()
	mux.HandleFunc("POST /exec", func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewDecoder(r.Body).Decode(&req)
		_ = json.NewEncoder(w).Encode(api.ExecResult{Stdout: "out", ExitCode: 2})
	})
	res, err := newTestClient(t, mux).Exec(ctx(), api.ExecRequest{Worktree: "w", Resource: "api", Cmd: []string{"ls"}})
	if err != nil {
		t.Fatal(err)
	}
	if req.Resource != "api" || len(req.Cmd) != 1 {
		t.Errorf("exec req = %+v", req)
	}
	if res.Stdout != "out" || res.ExitCode != 2 {
		t.Errorf("exec res = %+v", res)
	}
}

func TestErrorBodyParsed(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /status", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		_ = json.NewEncoder(w).Encode(map[string]string{"error": "boom"})
	})
	_, err := newTestClient(t, mux).Status(ctx())
	if err == nil || err.Error() != "boom" {
		t.Errorf("want error \"boom\", got %v", err)
	}
}

func TestErrorNoBody(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /status", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	})
	_, err := newTestClient(t, mux).Status(ctx())
	if err == nil || !contains(err.Error(), "daemon returned 404") {
		t.Errorf("want \"daemon returned 404\", got %v", err)
	}
}

func TestUnreachable(t *testing.T) {
	c := New(filepath.Join(t.TempDir(), "nonexistent.sock"))
	_, err := c.Status(ctx())
	if err == nil || !contains(err.Error(), "daemon unreachable") {
		t.Errorf("want \"daemon unreachable\", got %v", err)
	}
}

type upBody struct {
	Worktree string `json:"worktree"`
	Force    bool   `json:"force"`
}

type resourceBody struct {
	Worktree string `json:"worktree"`
	Resource string `json:"resource"`
}

func contains(s, sub string) bool {
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return true
		}
	}
	return false
}
