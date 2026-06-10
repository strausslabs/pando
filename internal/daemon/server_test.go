package daemon

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/guyStrauss/pando/internal/api"
	"github.com/guyStrauss/pando/internal/logbuf"
)

type fakeOps struct {
	upCalled    bool
	upForce     bool
	upWorktree  string
	execResult  api.ExecResult
	statusErr   error
	lastExecReq api.ExecRequest
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
	return f.execResult, nil
}
func (f *fakeOps) Up(_ context.Context, wt string, force bool) error {
	f.upCalled, f.upWorktree, f.upForce = true, wt, force
	return nil
}
func (f *fakeOps) Down(context.Context, string) error            { return nil }
func (f *fakeOps) Restart(context.Context, string, string) error { return nil }
func (f *fakeOps) Rebuild(context.Context, string, string) error { return nil }
func (f *fakeOps) Trigger(context.Context, string, string) error { return nil }
func (f *fakeOps) ListWorktrees(context.Context) ([]api.WorktreeInfo, error) {
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
