package daemon

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/coder/websocket"
	"github.com/coder/websocket/wsjson"
	"github.com/strausslabs/pando/internal/logbuf"
)

func TestToWireEventLog(t *testing.T) {
	ts := time.Unix(1700000000, 0)
	ev := logbuf.Event{
		Kind:     logbuf.EventLog,
		Worktree: "main",
		Resource: "api",
		Line:     &logbuf.Line{Seq: 5, Time: ts, Worktree: "main", Resource: "api", Stream: logbuf.Stdout, Text: "hi"},
	}
	we := toWireEvent(ev)
	if we.Kind != "log" || we.Worktree != "main" || we.Resource != "api" {
		t.Errorf("envelope wrong: %+v", we)
	}
	if we.Line == nil {
		t.Fatal("log event must carry a line")
	}
	if we.Line.Seq != 5 || we.Line.Stream != "stdout" || we.Line.Text != "hi" || !we.Line.Time.Equal(ts) {
		t.Errorf("line mapped wrong: %+v", we.Line)
	}
}

func TestToWireEventPhase(t *testing.T) {
	ev := logbuf.Event{Kind: logbuf.EventPhase, Worktree: "feat", Resource: "web", Phase: "healthy"}
	we := toWireEvent(ev)
	if we.Kind != "phase" || we.Phase != "healthy" {
		t.Errorf("phase envelope wrong: %+v", we)
	}
	if we.Line != nil {
		t.Errorf("phase event should have no line, got %+v", we.Line)
	}
}

func mk() logbuf.Line { return logbuf.Line{Time: time.Unix(1, 0)} }

func dialEvents(t *testing.T, base, query string) (*websocket.Conn, context.Context) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	t.Cleanup(cancel)
	url := "ws" + strings.TrimPrefix(base, "http") + "/events"
	if query != "" {
		url += "?" + query
	}
	c, resp, err := websocket.Dial(ctx, url, nil)
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	if resp != nil && resp.Body != nil {
		_ = resp.Body.Close()
	}
	t.Cleanup(func() { _ = c.CloseNow() })
	return c, ctx
}

func TestEventsStreamsLiveLog(t *testing.T) {
	s, _ := testServer()
	srv := httptest.NewServer(s.Handler())
	t.Cleanup(srv.Close)

	c, ctx := dialEvents(t, srv.URL, "")
	// Give handleEvents time to subscribe before the append.
	time.Sleep(50 * time.Millisecond)
	s.logs.Append("main", "api", logbuf.Stdout, "live", mk)

	var ev wireEvent
	if err := wsjson.Read(ctx, c, &ev); err != nil {
		t.Fatal(err)
	}
	if ev.Kind != "log" || ev.Line == nil || ev.Line.Text != "live" {
		t.Errorf("unexpected event: %+v", ev)
	}
}

func TestEventsWorktreeFilter(t *testing.T) {
	s, _ := testServer()
	srv := httptest.NewServer(s.Handler())
	t.Cleanup(srv.Close)

	c, ctx := dialEvents(t, srv.URL, "worktree=main")
	time.Sleep(50 * time.Millisecond)
	s.logs.Append("other", "api", logbuf.Stdout, "skip-me", mk)
	s.logs.Append("main", "api", logbuf.Stdout, "keep-me", mk)

	var ev wireEvent
	if err := wsjson.Read(ctx, c, &ev); err != nil {
		t.Fatal(err)
	}
	if ev.Line == nil || ev.Line.Text != "keep-me" {
		t.Errorf("filter should drop other-worktree line, got %+v", ev)
	}
}

func TestEventsReplay(t *testing.T) {
	s, _ := testServer()
	s.logs.Append("main", "api", logbuf.Stdout, "one", mk)
	s.logs.Append("main", "api", logbuf.Stdout, "two", mk)
	srv := httptest.NewServer(s.Handler())
	t.Cleanup(srv.Close)

	c, ctx := dialEvents(t, srv.URL, "lastSeq=1")
	var ev wireEvent
	if err := wsjson.Read(ctx, c, &ev); err != nil {
		t.Fatal(err)
	}
	if ev.Line == nil || ev.Line.Text != "two" {
		t.Errorf("replay should resend seq>1 (\"two\"), got %+v", ev)
	}
}

func TestEventsUnavailableWithoutLogStore(t *testing.T) {
	s := NewServer(&fakeOps{}, nil)
	rec := httptest.NewRecorder()
	s.Handler().ServeHTTP(rec, httptest.NewRequest("GET", "/events", nil))
	if rec.Code != http.StatusServiceUnavailable {
		t.Errorf("no log store should be 503, got %d", rec.Code)
	}
}
