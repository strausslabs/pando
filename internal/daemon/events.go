package daemon

import (
	"context"
	"net/http"
	"time"

	"github.com/coder/websocket"
	"github.com/coder/websocket/wsjson"
	"github.com/guyStrauss/pando/internal/api"
	"github.com/guyStrauss/pando/internal/logbuf"
)

func (s *Server) handleEvents(w http.ResponseWriter, r *http.Request) {
	if s.logs == nil {
		writeErr(w, http.StatusServiceUnavailable, errClosed)
		return
	}
	c, err := websocket.Accept(w, r, &websocket.AcceptOptions{
		OriginPatterns: []string{"*"},
	})
	if err != nil {
		return
	}
	defer func() { _ = c.CloseNow() }()

	ctx := c.CloseRead(r.Context())
	worktreeFilter := r.URL.Query().Get("worktree")

	subID, ch := s.logs.Subscribe(512)
	defer s.logs.Unsubscribe(subID)

	if lastSeq := atoiDefault(r.URL.Query().Get("lastSeq"), 0); lastSeq > 0 {
		s.replay(ctx, c, worktreeFilter, uint64(lastSeq))
	}

	pinger := time.NewTicker(30 * time.Second)
	defer pinger.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-pinger.C:
			pctx, cancel := context.WithTimeout(ctx, 10*time.Second)
			err := c.Ping(pctx)
			cancel()
			if err != nil {
				return
			}
		case ev, ok := <-ch:
			if !ok {
				return
			}
			if worktreeFilter != "" && ev.Worktree != worktreeFilter {
				continue
			}
			if err := wsjson.Write(ctx, c, toWireEvent(ev)); err != nil {
				return
			}
		}
	}
}

func (s *Server) replay(ctx context.Context, c *websocket.Conn, worktreeFilter string, afterSeq uint64) {
	for _, ref := range s.logs.Resources() {
		if worktreeFilter != "" && ref.Worktree != worktreeFilter {
			continue
		}
		lines, err := s.logs.Query(ref.Worktree, ref.Resource, logbuf.Query{AfterSeq: afterSeq})
		if err != nil {
			continue
		}
		for i := range lines {
			ev := logbuf.Event{Kind: logbuf.EventLog, Worktree: ref.Worktree, Resource: ref.Resource, Line: &lines[i]}
			if err := wsjson.Write(ctx, c, toWireEvent(ev)); err != nil {
				return
			}
		}
	}
}

type wireEvent struct {
	Kind     string       `json:"kind"`
	Worktree string       `json:"worktree"`
	Resource string       `json:"resource"`
	Phase    string       `json:"phase,omitempty"`
	Line     *api.LogLine `json:"line,omitempty"`
}

func toWireEvent(ev logbuf.Event) wireEvent {
	we := wireEvent{
		Kind:     string(ev.Kind),
		Worktree: ev.Worktree,
		Resource: ev.Resource,
		Phase:    ev.Phase,
	}
	if ev.Line != nil {
		we.Line = &api.LogLine{
			Seq:      ev.Line.Seq,
			Time:     ev.Line.Time,
			Worktree: ev.Line.Worktree,
			Resource: ev.Line.Resource,
			Stream:   string(ev.Line.Stream),
			Text:     ev.Line.Text,
		}
	}
	return we
}
