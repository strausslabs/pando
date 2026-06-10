package logbuf

import (
	"sort"
	"sync"
)

type EventKind string

const (
	EventLog   EventKind = "log"
	EventPhase EventKind = "phase"
)

type Event struct {
	Kind     EventKind `json:"kind"`
	Worktree string    `json:"worktree"`
	Resource string    `json:"resource"`
	Line     *Line     `json:"line,omitempty"`
	Phase    string    `json:"phase,omitempty"`
}

// Store holds one ring Buffer per (worktree, resource) and a pub/sub bus that
// fans every append out to live subscribers (UI sockets, MCP streams). One
// producer write reaches all readers; queriers hit the buffer directly.
type Store struct {
	mu       sync.RWMutex
	capacity int
	buffers  map[string]*Buffer

	subMu   sync.Mutex
	subs    map[int]chan Event
	nextSub int
}

func NewStore(capacity int) *Store {
	return &Store{
		capacity: capacity,
		buffers:  make(map[string]*Buffer),
		subs:     make(map[int]chan Event),
	}
}

func bufKey(worktree, resource string) string { return worktree + "\x00" + resource }

func (s *Store) buffer(worktree, resource string) *Buffer {
	key := bufKey(worktree, resource)
	s.mu.RLock()
	b, ok := s.buffers[key]
	s.mu.RUnlock()
	if ok {
		return b
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if b, ok = s.buffers[key]; ok {
		return b
	}
	b = New(s.capacity)
	s.buffers[key] = b
	return b
}

func (s *Store) Append(worktree, resource string, stream Stream, text string, now func() Line) {
	l := now()
	l.Worktree = worktree
	l.Resource = resource
	l.Stream = stream
	l.Text = text
	stored := s.buffer(worktree, resource).Append(l)
	s.publish(Event{Kind: EventLog, Worktree: worktree, Resource: resource, Line: &stored})
}

func (s *Store) Query(worktree, resource string, q Query) ([]Line, error) {
	return s.buffer(worktree, resource).Query(q)
}

// Text returns the plain text of all buffered lines for a resource, oldest
// first. Satisfies the logMatch probe's querier.
func (s *Store) Text(worktree, resource string) []string {
	lines, _ := s.buffer(worktree, resource).Query(Query{})
	out := make([]string, len(lines))
	for i, l := range lines {
		out[i] = l.Text
	}
	return out
}

func (s *Store) PublishPhase(worktree, resource, phase string) {
	s.publish(Event{Kind: EventPhase, Worktree: worktree, Resource: resource, Phase: phase})
}

func (s *Store) publish(e Event) {
	s.subMu.Lock()
	defer s.subMu.Unlock()
	for _, ch := range s.subs {
		// Drop on slow consumer rather than block the producer; UI will catch up
		// via a subsequent Query on reconnect.
		select {
		case ch <- e:
		default:
		}
	}
}

func (s *Store) Subscribe(buffer int) (int, <-chan Event) {
	if buffer < 1 {
		buffer = 256
	}
	ch := make(chan Event, buffer)
	s.subMu.Lock()
	defer s.subMu.Unlock()
	id := s.nextSub
	s.nextSub++
	s.subs[id] = ch
	return id, ch
}

func (s *Store) Unsubscribe(id int) {
	s.subMu.Lock()
	defer s.subMu.Unlock()
	if ch, ok := s.subs[id]; ok {
		delete(s.subs, id)
		close(ch)
	}
}

type ResourceRef struct {
	Worktree string
	Resource string
}

func (s *Store) Resources() []ResourceRef {
	s.mu.RLock()
	defer s.mu.RUnlock()
	refs := make([]ResourceRef, 0, len(s.buffers))
	for key := range s.buffers {
		parts := splitKey(key)
		refs = append(refs, ResourceRef{Worktree: parts[0], Resource: parts[1]})
	}
	sort.Slice(refs, func(i, j int) bool {
		if refs[i].Worktree != refs[j].Worktree {
			return refs[i].Worktree < refs[j].Worktree
		}
		return refs[i].Resource < refs[j].Resource
	})
	return refs
}

func splitKey(key string) [2]string {
	for i := 0; i < len(key); i++ {
		if key[i] == 0 {
			return [2]string{key[:i], key[i+1:]}
		}
	}
	return [2]string{key, ""}
}
