package daemon

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/strausslabs/pando/internal/api"
	"github.com/strausslabs/pando/internal/logbuf"
	"github.com/strausslabs/pando/internal/selfupdate"
)

type fakeOps struct {
	upCalled     bool
	upForce      bool
	upWorktree   string
	execResult   api.ExecResult
	statusErr    error
	worktreesErr error
	upErr        error
	downErr      error
	actionErr    error
	execErr      error
	lastExecReq  api.ExecRequest
}

func (f *fakeOps) Status(context.Context) ([]api.WorktreeStatus, error) {
	if f.statusErr != nil {
		return nil, f.statusErr
	}
	return []api.WorktreeStatus{{Worktree: "main", Resources: []api.ResourceStatus{{Name: "api", Phase: "healthy"}}}}, nil
}
func (f *fakeOps) Logs(context.Context, api.LogQuery) ([]api.LogLine, error) {
	return []api.LogLine{{Text: "hello", Resource: "api"}}, nil
}
func (f *fakeOps) Exec(_ context.Context, req api.ExecRequest) (api.ExecResult, error) {
	f.lastExecReq = req
	return f.execResult, f.execErr
}
func (f *fakeOps) Up(_ context.Context, wt string, force bool) error {
	f.upCalled, f.upWorktree, f.upForce = true, wt, force
	return f.upErr
}
func (f *fakeOps) Down(context.Context, string) error            { return f.downErr }
func (f *fakeOps) Restart(context.Context, string, string) error { return f.actionErr }
func (f *fakeOps) Rebuild(context.Context, string, string) error { return f.actionErr }
func (f *fakeOps) Trigger(context.Context, string, string) error { return f.actionErr }
func (f *fakeOps) ListWorktrees(context.Context) ([]api.WorktreeInfo, error) {
	if f.worktreesErr != nil {
		return nil, f.worktreesErr
	}
	return []api.WorktreeInfo{{Slug: "main", Branch: "main", Ports: map[string]int{"api": 8001}}}, nil
}

func testServer() (*Server, *fakeOps) {
	ops := &fakeOps{}
	return NewServer(ops, logbuf.NewStore(100)), ops
}

func TestHealthz(t *testing.T) {
	s, _ := testServer()
	rec := httptest.NewRecorder()
	s.Handler().ServeHTTP(rec, httptest.NewRequest("GET", "/healthz", nil))
	if rec.Code != 200 {
		t.Fatalf("healthz code %d", rec.Code)
	}
}

func TestStatusEndpoint(t *testing.T) {
	s, _ := testServer()
	rec := httptest.NewRecorder()
	s.Handler().ServeHTTP(rec, httptest.NewRequest("GET", "/status", nil))
	if rec.Code != 200 {
		t.Fatalf("status code %d", rec.Code)
	}
	var got []api.WorktreeStatus
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 || got[0].Worktree != "main" {
		t.Errorf("unexpected status: %+v", got)
	}
}

func TestUpDecodesForce(t *testing.T) {
	s, ops := testServer()
	rec := httptest.NewRecorder()
	body := strings.NewReader(`{"worktree":"feat-x","force":true}`)
	s.Handler().ServeHTTP(rec, httptest.NewRequest("POST", "/up", body))
	if rec.Code != 200 {
		t.Fatalf("up code %d body %s", rec.Code, rec.Body)
	}
	if !ops.upCalled || ops.upWorktree != "feat-x" || !ops.upForce {
		t.Errorf("up not called correctly: called=%v wt=%q force=%v", ops.upCalled, ops.upWorktree, ops.upForce)
	}
}

func TestExecRoundTrip(t *testing.T) {
	s, ops := testServer()
	ops.execResult = api.ExecResult{Stdout: "out", ExitCode: 2}
	rec := httptest.NewRecorder()
	body := strings.NewReader(`{"worktree":"main","resource":"api","cmd":["ls","-la"]}`)
	s.Handler().ServeHTTP(rec, httptest.NewRequest("POST", "/exec", body))
	if rec.Code != 200 {
		t.Fatalf("exec code %d", rec.Code)
	}
	var res api.ExecResult
	_ = json.Unmarshal(rec.Body.Bytes(), &res)
	if res.Stdout != "out" || res.ExitCode != 2 {
		t.Errorf("exec result wrong: %+v", res)
	}
	if len(ops.lastExecReq.Cmd) != 2 || ops.lastExecReq.Cmd[0] != "ls" {
		t.Errorf("exec cmd not decoded: %+v", ops.lastExecReq.Cmd)
	}
}

func TestLogsQueryParams(t *testing.T) {
	s, _ := testServer()
	rec := httptest.NewRecorder()
	s.Handler().ServeHTTP(rec, httptest.NewRequest("GET", "/logs?worktree=main&resource=api&tail=50", nil))
	if rec.Code != 200 {
		t.Fatalf("logs code %d", rec.Code)
	}
	var lines []api.LogLine
	_ = json.Unmarshal(rec.Body.Bytes(), &lines)
	if len(lines) != 1 || lines[0].Text != "hello" {
		t.Errorf("logs wrong: %+v", lines)
	}
}

func TestBadJSONRejected(t *testing.T) {
	s, _ := testServer()
	rec := httptest.NewRecorder()
	s.Handler().ServeHTTP(rec, httptest.NewRequest("POST", "/up", strings.NewReader("{not json")))
	if rec.Code != http.StatusBadRequest {
		t.Errorf("bad json should be 400, got %d", rec.Code)
	}
}

func TestStatusErrorPropagates(t *testing.T) {
	ops := &fakeOps{statusErr: context.DeadlineExceeded}
	s := NewServer(ops, nil)
	rec := httptest.NewRecorder()
	s.Handler().ServeHTTP(rec, httptest.NewRequest("GET", "/status", nil))
	if rec.Code != http.StatusInternalServerError {
		t.Errorf("expected 500, got %d", rec.Code)
	}
}

func TestVersionDefaultThenSet(t *testing.T) {
	s, _ := testServer()

	rec := httptest.NewRecorder()
	s.Handler().ServeHTTP(rec, httptest.NewRequest("GET", "/version", nil))
	var st api.UpdateStatus
	_ = json.Unmarshal(rec.Body.Bytes(), &st)
	if st.Available || st.Current != "" {
		t.Errorf("default version should be empty/unavailable, got %+v", st)
	}

	s.SetUpdate(selfupdate.Status{Current: "v1", Latest: "v2", Available: true})
	rec = httptest.NewRecorder()
	s.Handler().ServeHTTP(rec, httptest.NewRequest("GET", "/version", nil))
	_ = json.Unmarshal(rec.Body.Bytes(), &st)
	if !st.Available || st.Current != "v1" || st.Latest != "v2" {
		t.Errorf("after SetUpdate, got %+v", st)
	}
}

func TestWorktreesEndpoint(t *testing.T) {
	s, _ := testServer()
	rec := httptest.NewRecorder()
	s.Handler().ServeHTTP(rec, httptest.NewRequest("GET", "/worktrees", nil))
	if rec.Code != 200 {
		t.Fatalf("worktrees code %d", rec.Code)
	}
	var got []api.WorktreeInfo
	_ = json.Unmarshal(rec.Body.Bytes(), &got)
	if len(got) != 1 || got[0].Slug != "main" {
		t.Errorf("worktrees = %+v", got)
	}
}

func TestWorktreesErrorPropagates(t *testing.T) {
	s := NewServer(&fakeOps{worktreesErr: context.DeadlineExceeded}, nil)
	rec := httptest.NewRecorder()
	s.Handler().ServeHTTP(rec, httptest.NewRequest("GET", "/worktrees", nil))
	if rec.Code != http.StatusInternalServerError {
		t.Errorf("expected 500, got %d", rec.Code)
	}
}

func TestDownEndpoint(t *testing.T) {
	s, _ := testServer()
	rec := httptest.NewRecorder()
	s.Handler().ServeHTTP(rec, httptest.NewRequest("POST", "/down", strings.NewReader(`{"worktree":"x"}`)))
	if rec.Code != 200 {
		t.Errorf("down code %d", rec.Code)
	}
}

func TestDownErrorPropagates(t *testing.T) {
	s := NewServer(&fakeOps{downErr: context.DeadlineExceeded}, nil)
	rec := httptest.NewRecorder()
	s.Handler().ServeHTTP(rec, httptest.NewRequest("POST", "/down", strings.NewReader(`{"worktree":"x"}`)))
	if rec.Code != http.StatusInternalServerError {
		t.Errorf("expected 500, got %d", rec.Code)
	}
}

func TestResourceActionsEndpoints(t *testing.T) {
	for _, path := range []string{"/restart", "/rebuild", "/trigger"} {
		t.Run(path, func(t *testing.T) {
			s, _ := testServer()
			rec := httptest.NewRecorder()
			body := strings.NewReader(`{"worktree":"w","resource":"api"}`)
			s.Handler().ServeHTTP(rec, httptest.NewRequest("POST", path, body))
			if rec.Code != 200 {
				t.Errorf("%s code %d", path, rec.Code)
			}
		})
	}
}

func TestResourceActionErrorPropagates(t *testing.T) {
	s := NewServer(&fakeOps{actionErr: context.DeadlineExceeded}, nil)
	rec := httptest.NewRecorder()
	body := strings.NewReader(`{"worktree":"w","resource":"api"}`)
	s.Handler().ServeHTTP(rec, httptest.NewRequest("POST", "/restart", body))
	if rec.Code != http.StatusInternalServerError {
		t.Errorf("expected 500, got %d", rec.Code)
	}
}

func TestExecErrorPropagates(t *testing.T) {
	s := NewServer(&fakeOps{execErr: context.DeadlineExceeded}, nil)
	rec := httptest.NewRecorder()
	body := strings.NewReader(`{"worktree":"w","resource":"api","cmd":["ls"]}`)
	s.Handler().ServeHTTP(rec, httptest.NewRequest("POST", "/exec", body))
	if rec.Code != http.StatusInternalServerError {
		t.Errorf("expected 500, got %d", rec.Code)
	}
}

func TestUpErrorPropagates(t *testing.T) {
	s := NewServer(&fakeOps{upErr: context.DeadlineExceeded}, nil)
	rec := httptest.NewRecorder()
	s.Handler().ServeHTTP(rec, httptest.NewRequest("POST", "/up", strings.NewReader(`{"worktree":"x"}`)))
	if rec.Code != http.StatusInternalServerError {
		t.Errorf("up error expected 500, got %d", rec.Code)
	}
}

func TestMountUI(t *testing.T) {
	s, _ := testServer()
	s.MountUI(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte("ui-root"))
	}))
	rec := httptest.NewRecorder()
	s.Handler().ServeHTTP(rec, httptest.NewRequest("GET", "/", nil))
	if rec.Body.String() != "ui-root" {
		t.Errorf("MountUI not serving root, got %q", rec.Body.String())
	}
}

func TestLogsSinceAndBadTail(t *testing.T) {
	s, _ := testServer()
	rec := httptest.NewRecorder()
	s.Handler().ServeHTTP(rec, httptest.NewRequest("GET", "/logs?worktree=main&resource=api&since=30s&tail=abc", nil))
	if rec.Code != 200 {
		t.Fatalf("logs code %d", rec.Code)
	}
}

func TestAtoiDefault(t *testing.T) {
	if got := atoiDefault("50", 0); got != 50 {
		t.Errorf("atoiDefault(50) = %d", got)
	}
	if got := atoiDefault("abc", 7); got != 7 {
		t.Errorf("atoiDefault(abc) = %d, want default 7", got)
	}
	// Empty string has no non-digit to trip the default; the loop is skipped and
	// the accumulator (0) is returned.
	if got := atoiDefault("", 3); got != 0 {
		t.Errorf("atoiDefault(empty) = %d, want 0", got)
	}
}
