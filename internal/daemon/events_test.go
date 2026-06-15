package daemon

import (
	"testing"
	"time"

	"github.com/guyStrauss/pando/internal/logbuf"
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
