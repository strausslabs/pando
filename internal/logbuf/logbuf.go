package logbuf

import (
	"regexp"
	"sync"
	"time"
)

type Stream string

const (
	Stdout Stream = "stdout"
	Stderr Stream = "stderr"
	System Stream = "system"
)

type Line struct {
	Seq      uint64    `json:"seq"`
	Time     time.Time `json:"time"`
	Worktree string    `json:"worktree"`
	Resource string    `json:"resource"`
	Stream   Stream    `json:"stream"`
	Text     string    `json:"text"`
}

type Buffer struct {
	mu    sync.RWMutex
	cap   int
	lines []Line
	start int
	count int
	seq   uint64
}

func New(capacity int) *Buffer {
	if capacity < 1 {
		capacity = 1
	}
	return &Buffer{cap: capacity, lines: make([]Line, capacity)}
}

func (b *Buffer) Append(l Line) Line {
	b.mu.Lock()
	defer b.mu.Unlock()
	if l.Seq == 0 {
		b.seq++
		l.Seq = b.seq
	}
	idx := (b.start + b.count) % b.cap
	if b.count == b.cap {
		b.start = (b.start + 1) % b.cap
		idx = (b.start + b.count - 1) % b.cap
	} else {
		b.count++
	}
	b.lines[idx] = l
	return l
}

type Query struct {
	Tail     int
	Since    time.Time
	AfterSeq uint64
	Grep     string
}

func (b *Buffer) Query(q Query) ([]Line, error) {
	var re *regexp.Regexp
	if q.Grep != "" {
		var err error
		re, err = regexp.Compile(q.Grep)
		if err != nil {
			return nil, err
		}
	}
	b.mu.RLock()
	defer b.mu.RUnlock()

	out := make([]Line, 0, b.count)
	for i := 0; i < b.count; i++ {
		l := b.lines[(b.start+i)%b.cap]
		if q.AfterSeq > 0 && l.Seq <= q.AfterSeq {
			continue
		}
		if !q.Since.IsZero() && l.Time.Before(q.Since) {
			continue
		}
		if re != nil && !re.MatchString(l.Text) {
			continue
		}
		out = append(out, l)
	}
	if q.Tail > 0 && len(out) > q.Tail {
		out = out[len(out)-q.Tail:]
	}
	return out, nil
}

func (b *Buffer) Len() int {
	b.mu.RLock()
	defer b.mu.RUnlock()
	return b.count
}
