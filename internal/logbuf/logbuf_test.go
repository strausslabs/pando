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

func TestStoreText(t *testing.T) {
	s := NewStore(10)
	mk := func() Line { return Line{Time: time.Unix(1, 0)} }
	s.Append("main", "api", Stdout, "first", mk)
	s.Append("main", "api", Stderr, "second", mk)
	got := s.Text("main", "api")
	if len(got) != 2 || got[0] != "first" || got[1] != "second" {
		t.Errorf("Text = %v, want [first second]", got)
	}
	if other := s.Text("main", "absent"); len(other) != 0 {
		t.Errorf("Text for unknown resource should be empty, got %v", other)
	}
}

func TestStorePublishPhase(t *testing.T) {
	s := NewStore(10)
	_, ch := s.Subscribe(4)
	s.PublishPhase("feat", "web", "healthy")
	select {
	case e := <-ch:
		if e.Kind != EventPhase || e.Worktree != "feat" || e.Resource != "web" || e.Phase != "healthy" {
			t.Errorf("bad phase event: %+v", e)
		}
		if e.Line != nil {
			t.Errorf("phase event should carry no line, got %+v", e.Line)
		}
	case <-time.After(time.Second):
		t.Fatal("no phase event received")
	}
}

func TestSplitKey(t *testing.T) {
	if got := splitKey("main\x00api"); got != [2]string{"main", "api"} {
		t.Errorf("splitKey = %v, want [main api]", got)
	}
	if got := splitKey("main\x00"); got != [2]string{"main", ""} {
		t.Errorf("splitKey empty resource = %v", got)
	}
}

// TestStoreGlobalSeqUniqueAcrossResources is the whole point of moving the seq
// counter to the Store: interleaving appends across two resources (in two
// worktrees) must hand out globally unique Seq values so the merged
// all-resources view does not collide rows. A per-buffer counter would give
// both resources 1,2,3,...
func TestStoreGlobalSeqUniqueAcrossResources(t *testing.T) {
	s := NewStore(100)
	mk := func() Line { return Line{Time: time.Unix(1, 0)} }

	// Interleave across two resources and two worktrees on the SAME store.
	s.Append("main", "api", Stdout, "a", mk)
	s.Append("main", "web", Stdout, "b", mk)
	s.Append("feat", "api", Stderr, "c", mk)
	s.Append("main", "api", System, "d", mk)
	s.Append("main", "web", Stdout, "e", mk)
	s.Append("feat", "api", Stdout, "f", mk)

	refs := []ResourceRef{
		{Worktree: "main", Resource: "api"},
		{Worktree: "main", Resource: "web"},
		{Worktree: "feat", Resource: "api"},
	}
	seen := map[uint64]string{}
	total := 0
	for _, ref := range refs {
		lines, err := s.Query(ref.Worktree, ref.Resource, Query{})
		if err != nil {
			t.Fatal(err)
		}
		for _, l := range lines {
			total++
			where := ref.Worktree + "/" + ref.Resource
			if prev, ok := seen[l.Seq]; ok {
				t.Errorf("seq %d reused across resources: %s and %s", l.Seq, prev, where)
			}
			seen[l.Seq] = where
			if l.Seq == 0 {
				t.Errorf("store should stamp a non-zero global seq, got 0 in %s", where)
			}
		}
	}
	if total != 6 {
		t.Fatalf("expected 6 lines total across resources, got %d", total)
	}
	if len(seen) != 6 {
		t.Errorf("expected 6 distinct global seqs, got %d: %v", len(seen), seen)
	}
}

// TestStoreGlobalSeqMonotonicInAppendOrder asserts that each successive Append,
// regardless of which resource it targets, gets a Seq strictly greater than the
// previous one. The Store appends serially here, so ordering is deterministic.
func TestStoreGlobalSeqMonotonicInAppendOrder(t *testing.T) {
	s := NewStore(100)
	mk := func() Line { return Line{Time: time.Unix(1, 0)} }

	// Alternate resources so a per-buffer counter would visibly reset.
	appends := []struct{ wt, res, text string }{
		{"main", "api", "1"},
		{"main", "web", "2"},
		{"main", "api", "3"},
		{"feat", "db", "4"},
		{"main", "web", "5"},
		{"feat", "db", "6"},
		{"main", "api", "7"},
	}
	for _, a := range appends {
		s.Append(a.wt, a.res, Stdout, a.text, mk)
	}

	byText := map[string]uint64{}
	for _, ref := range s.Resources() {
		lines, _ := s.Query(ref.Worktree, ref.Resource, Query{})
		for _, l := range lines {
			byText[l.Text] = l.Seq
		}
	}
	var prev uint64
	for i, a := range appends {
		seq := byText[a.text]
		if i > 0 && seq <= prev {
			t.Errorf("seq not strictly increasing in append order at %d: %d after %d", i, seq, prev)
		}
		prev = seq
	}
}

// TestStoreGlobalSeqConcurrentNoDuplicates fires many concurrent appends across
// a mix of resources and asserts that the multiset of stored Seqs has no
// duplicates and that the total count matches the number of appends.
func TestStoreGlobalSeqConcurrentNoDuplicates(t *testing.T) {
	s := NewStore(100000)
	mk := func() Line { return Line{Time: time.Unix(1, 0)} }

	const goroutines = 16
	const perG = 500
	resources := []ResourceRef{
		{"main", "api"},
		{"main", "web"},
		{"feat", "api"},
		{"feat", "db"},
	}

	var wg sync.WaitGroup
	for g := 0; g < goroutines; g++ {
		g := g
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := 0; j < perG; j++ {
				ref := resources[(g+j)%len(resources)]
				s.Append(ref.Worktree, ref.Resource, Stdout, "x", mk)
			}
		}()
	}
	wg.Wait()

	want := goroutines * perG
	seen := map[uint64]bool{}
	total := 0
	for _, ref := range resources {
		lines, err := s.Query(ref.Worktree, ref.Resource, Query{})
		if err != nil {
			t.Fatal(err)
		}
		for _, l := range lines {
			total++
			if seen[l.Seq] {
				t.Errorf("duplicate global seq %d under concurrency", l.Seq)
			}
			seen[l.Seq] = true
		}
	}
	if total != want {
		t.Errorf("expected %d total lines, got %d", want, total)
	}
	if len(seen) != want {
		t.Errorf("expected %d unique global seqs, got %d", want, len(seen))
	}
}

// TestBufferAppendPreservesPresetSeq documents the mechanism the Store relies on:
// Buffer.Append keeps a pre-assigned non-zero Seq (so the Store's global counter
// survives) and only falls back to the buffer-local counter when Seq==0.
func TestBufferAppendPreservesPresetSeq(t *testing.T) {
	b := New(10)

	got := b.Append(Line{Seq: 42, Text: "preset"})
	if got.Seq != 42 {
		t.Errorf("pre-set seq must be preserved, got %d", got.Seq)
	}

	// A zero-seq append takes the buffer-local counter, independent of the
	// preset value above.
	got = b.Append(Line{Text: "auto"})
	if got.Seq != 1 {
		t.Errorf("zero-seq append should get buffer-local seq 1, got %d", got.Seq)
	}

	got = b.Append(Line{Seq: 7, Text: "preset2"})
	if got.Seq != 7 {
		t.Errorf("second pre-set seq must be preserved, got %d", got.Seq)
	}

	got = b.Append(Line{Text: "auto2"})
	if got.Seq != 2 {
		t.Errorf("buffer-local counter should advance independently to 2, got %d", got.Seq)
	}
}
