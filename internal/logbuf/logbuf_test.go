package logbuf

import (
	"sync"
	"testing"
	"time"
)

func at(sec int) func() Line {
	return func() Line { return Line{Time: time.Unix(int64(sec), 0)} }
}

func TestRingEviction(t *testing.T) {
	b := New(3)
	for i := 0; i < 5; i++ {
		b.Append(Line{Text: string(rune('a' + i))})
	}
	lines, _ := b.Query(Query{})
	if len(lines) != 3 {
		t.Fatalf("want 3 retained, got %d", len(lines))
	}
	if lines[0].Text != "c" || lines[2].Text != "e" {
		t.Errorf("oldest not evicted: %v", texts(lines))
	}
}

func TestSeqMonotonic(t *testing.T) {
	b := New(2)
	var seqs []uint64
	for i := 0; i < 4; i++ {
		l := b.Append(Line{Text: "x"})
		seqs = append(seqs, l.Seq)
	}
	for i := 1; i < len(seqs); i++ {
		if seqs[i] != seqs[i-1]+1 {
			t.Errorf("seq not monotonic: %v", seqs)
		}
	}
}

func TestQueryTail(t *testing.T) {
	b := New(10)
	for i := 0; i < 6; i++ {
		b.Append(Line{Text: string(rune('0' + i))})
	}
	lines, _ := b.Query(Query{Tail: 2})
	if len(lines) != 2 || lines[0].Text != "4" || lines[1].Text != "5" {
		t.Errorf("tail wrong: %v", texts(lines))
	}
}

func TestQuerySince(t *testing.T) {
	b := New(10)
	b.Append(at(10)())
	b.Append(at(20)())
	b.Append(at(30)())
	lines, _ := b.Query(Query{Since: time.Unix(20, 0)})
	if len(lines) != 2 {
		t.Errorf("since filter wrong, got %d", len(lines))
	}
}

func TestQueryAfterSeq(t *testing.T) {
	b := New(10)
	var third uint64
	for i := 0; i < 5; i++ {
		l := b.Append(Line{Text: "x"})
		if i == 2 {
			third = l.Seq
		}
	}
	lines, _ := b.Query(Query{AfterSeq: third})
	if len(lines) != 2 {
		t.Errorf("afterSeq should return lines 4,5; got %d", len(lines))
	}
}

func TestQueryGrep(t *testing.T) {
	b := New(10)
	b.Append(Line{Text: "INFO ok"})
	b.Append(Line{Text: "ERROR boom"})
	b.Append(Line{Text: "INFO fine"})
	lines, err := b.Query(Query{Grep: "ERROR"})
	if err != nil {
		t.Fatal(err)
	}
	if len(lines) != 1 || lines[0].Text != "ERROR boom" {
		t.Errorf("grep wrong: %v", texts(lines))
	}
}

func TestQueryBadRegex(t *testing.T) {
	b := New(2)
	if _, err := b.Query(Query{Grep: "("}); err == nil {
		t.Error("invalid regex should error")
	}
}

func TestConcurrentAppend(t *testing.T) {
	b := New(1000)
	var wg sync.WaitGroup
	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := 0; j < 100; j++ {
				b.Append(Line{Text: "x"})
			}
		}()
	}
	wg.Wait()
	if b.Len() != 1000 {
		t.Errorf("want 1000 lines, got %d", b.Len())
	}
}

func TestStoreFanout(t *testing.T) {
	s := NewStore(100)
	_, ch1 := s.Subscribe(10)
	_, ch2 := s.Subscribe(10)
	s.Append("main", "api", Stdout, "hello", func() Line { return Line{Time: time.Unix(1, 0)} })

	for _, ch := range []<-chan Event{ch1, ch2} {
		select {
		case e := <-ch:
			if e.Kind != EventLog || e.Line.Text != "hello" {
				t.Errorf("bad event: %+v", e)
			}
		case <-time.After(time.Second):
			t.Fatal("subscriber did not receive event")
		}
	}
}

func TestStoreUnsubscribe(t *testing.T) {
	s := NewStore(100)
	id, ch := s.Subscribe(10)
	s.Unsubscribe(id)
	if _, ok := <-ch; ok {
		t.Error("channel should be closed after unsubscribe")
	}
}

func TestStorePerResourceIsolation(t *testing.T) {
	s := NewStore(100)
	mk := func() Line { return Line{Time: time.Unix(1, 0)} }
	s.Append("main", "api", Stdout, "a", mk)
	s.Append("feat", "api", Stdout, "b", mk)
	s.Append("main", "api", Stdout, "c", mk)

	lines, _ := s.Query("main", "api", Query{})
	if len(lines) != 2 {
		t.Errorf("main/api should have 2 lines, got %d", len(lines))
	}
	lines, _ = s.Query("feat", "api", Query{})
	if len(lines) != 1 {
		t.Errorf("feat/api should have 1 line, got %d", len(lines))
	}
}

func TestStoreSlowConsumerDropsNotBlocks(t *testing.T) {
	s := NewStore(100)
	s.Subscribe(1)
	done := make(chan struct{})
	go func() {
		for i := 0; i < 100; i++ {
			s.Append("main", "api", Stdout, "x", func() Line { return Line{Time: time.Unix(1, 0)} })
		}
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("producer blocked on slow consumer")
	}
}

func TestStoreResourcesSorted(t *testing.T) {
	s := NewStore(10)
	mk := func() Line { return Line{Time: time.Unix(1, 0)} }
	s.Append("main", "web", Stdout, "x", mk)
	s.Append("main", "api", Stdout, "x", mk)
	refs := s.Resources()
	if len(refs) != 2 || refs[0].Resource != "api" {
		t.Errorf("resources not sorted: %+v", refs)
	}
}

func texts(lines []Line) []string {
	out := make([]string, len(lines))
	for i, l := range lines {
		out[i] = l.Text
	}
	return out
}
