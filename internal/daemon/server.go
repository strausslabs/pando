package daemon

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"sync/atomic"
	"time"

	"github.com/strausslabs/pando/internal/api"
	"github.com/strausslabs/pando/internal/logbuf"
	"github.com/strausslabs/pando/internal/selfupdate"
)

var errClosed = errors.New("server closed")

type Server struct {
	ops    api.StackOps
	logs   *logbuf.Store
	mux    *http.ServeMux
	update atomic.Pointer[selfupdate.Status]
}

func NewServer(ops api.StackOps, logs *logbuf.Store) *Server {
	s := &Server{ops: ops, logs: logs, mux: http.NewServeMux()}
	s.routes()
	return s
}

func (s *Server) SetUpdate(st selfupdate.Status) { s.update.Store(&st) }

func (s *Server) Handler() http.Handler { return s.mux }

func (s *Server) routes() {
	s.mux.HandleFunc("GET /healthz", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
	})
	s.mux.HandleFunc("GET /version", s.handleVersion)
	s.mux.HandleFunc("GET /status", s.handleStatus)
	s.mux.HandleFunc("GET /worktrees", s.handleWorktrees)
	s.mux.HandleFunc("GET /logs", s.handleLogs)
	s.mux.HandleFunc("POST /up", s.handleUp)
	s.mux.HandleFunc("POST /down", s.handleDown)
	s.mux.HandleFunc("POST /restart", s.handleResourceAction(s.ops.Restart))
	s.mux.HandleFunc("POST /rebuild", s.handleResourceAction(s.ops.Rebuild))
	s.mux.HandleFunc("POST /trigger", s.handleResourceAction(s.ops.Trigger))
	s.mux.HandleFunc("POST /exec", s.handleExec)
	s.mux.HandleFunc("GET /events", s.handleEvents)
}

func (s *Server) MountUI(ui http.Handler) {
	s.mux.Handle("/", ui)
}

func (s *Server) handleVersion(w http.ResponseWriter, r *http.Request) {
	if st := s.update.Load(); st != nil {
		writeJSON(w, http.StatusOK, st)
		return
	}
	writeJSON(w, http.StatusOK, selfupdate.Status{})
}

func (s *Server) handleStatus(w http.ResponseWriter, r *http.Request) {
	st, err := s.ops.Status(r.Context())
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err)
		return
	}
	writeJSON(w, http.StatusOK, st)
}

func (s *Server) handleWorktrees(w http.ResponseWriter, r *http.Request) {
	wts, err := s.ops.ListWorktrees(r.Context())
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err)
		return
	}
	writeJSON(w, http.StatusOK, wts)
}

func (s *Server) handleLogs(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	query := api.LogQuery{
		Worktree: q.Get("worktree"),
		Resource: q.Get("resource"),
		Grep:     q.Get("grep"),
	}
	if tail := q.Get("tail"); tail != "" {
		query.Tail = atoiDefault(tail, 0)
	}
	if since := q.Get("since"); since != "" {
		if d, err := time.ParseDuration(since); err == nil {
			query.Since = time.Now().Add(-d)
		}
	}
	lines, err := s.ops.Logs(r.Context(), query)
	if err != nil {
		writeErr(w, http.StatusBadRequest, err)
		return
	}
	writeJSON(w, http.StatusOK, lines)
}

type upRequest struct {
	Worktree string `json:"worktree"`
	Force    bool   `json:"force"`
}

func (s *Server) handleUp(w http.ResponseWriter, r *http.Request) {
	var req upRequest
	if !decode(w, r, &req) {
		return
	}
	if err := s.ops.Up(r.Context(), req.Worktree, req.Force); err != nil {
		writeErr(w, http.StatusInternalServerError, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
}

type worktreeRequest struct {
	Worktree string `json:"worktree"`
}

func (s *Server) handleDown(w http.ResponseWriter, r *http.Request) {
	var req worktreeRequest
	if !decode(w, r, &req) {
		return
	}
	if err := s.ops.Down(r.Context(), req.Worktree); err != nil {
		writeErr(w, http.StatusInternalServerError, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
}

type resourceRequest struct {
	Worktree string `json:"worktree"`
	Resource string `json:"resource"`
}

func (s *Server) handleResourceAction(action func(context.Context, string, string) error) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var req resourceRequest
		if !decode(w, r, &req) {
			return
		}
		if err := action(r.Context(), req.Worktree, req.Resource); err != nil {
			writeErr(w, http.StatusInternalServerError, err)
			return
		}
		writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
	}
}

func (s *Server) handleExec(w http.ResponseWriter, r *http.Request) {
	var req api.ExecRequest
	if !decode(w, r, &req) {
		return
	}
	res, err := s.ops.Exec(r.Context(), req)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err)
		return
	}
	writeJSON(w, http.StatusOK, res)
}

func decode(w http.ResponseWriter, r *http.Request, v any) bool {
	if err := json.NewDecoder(r.Body).Decode(v); err != nil {
		writeErr(w, http.StatusBadRequest, err)
		return false
	}
	return true
}

func writeJSON(w http.ResponseWriter, code int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	_ = json.NewEncoder(w).Encode(v)
}

func writeErr(w http.ResponseWriter, code int, err error) {
	writeJSON(w, code, map[string]string{"error": err.Error()})
}

func atoiDefault(s string, def int) int {
	n := 0
	for _, c := range s {
		if c < '0' || c > '9' {
			return def
		}
		n = n*10 + int(c-'0')
	}
	return n
}
