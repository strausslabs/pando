package compose

import (
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/guyStrauss/pando/internal/logbuf"
)

type capturedLine struct {
	worktree string
	resource string
	stream   logbuf.Stream
	text     string
	at       time.Time
}

type fakeSink struct {
	mu    sync.Mutex
	lines []capturedLine
}

func (f *fakeSink) Append(worktree, resource string, stream logbuf.Stream, text string, mk func() logbuf.Line) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.lines = append(f.lines, capturedLine{worktree, resource, stream, text, mk().Time})
}

func TestPipeScansLines(t *testing.T) {
	fixed := time.Unix(1700000000, 0)
	sink := &fakeSink{}
	b := &Backend{sink: sink, clock: func() time.Time { return fixed }}

	b.pipe("feat-x", "api", logbuf.Stdout, strings.NewReader("a\nb\nc\n"))

	if len(sink.lines) != 3 {
		t.Fatalf("want 3 lines, got %d: %+v", len(sink.lines), sink.lines)
	}
	for i, want := range []string{"a", "b", "c"} {
		got := sink.lines[i]
		if got.text != want || got.worktree != "feat-x" || got.resource != "api" || got.stream != logbuf.Stdout {
			t.Errorf("line %d = %+v, want text %q", i, got, want)
		}
		if !got.at.Equal(fixed) {
			t.Errorf("line %d clock = %v, want %v", i, got.at, fixed)
		}
	}
}

func TestPipeKeepsLongLine(t *testing.T) {
	sink := &fakeSink{}
	b := &Backend{sink: sink, clock: time.Now}
	long := strings.Repeat("x", 900*1024)
	b.pipe("w", "r", logbuf.Stderr, strings.NewReader(long+"\n"))
	if len(sink.lines) != 1 {
		t.Fatalf("want 1 line, got %d", len(sink.lines))
	}
	if sink.lines[0].text != long {
		t.Errorf("long line truncated: got %d bytes, want %d", len(sink.lines[0].text), len(long))
	}
}
