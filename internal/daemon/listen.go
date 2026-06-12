package daemon

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"time"
)

func DefaultSocketPath() string {
	if dir := os.Getenv("XDG_RUNTIME_DIR"); dir != "" {
		return filepath.Join(dir, "pando.sock")
	}
	return filepath.Join(os.TempDir(), fmt.Sprintf("pando-%d.sock", os.Getuid()))
}

func (s *Server) Serve(ctx context.Context, socketPath string) error {
	if err := removeStaleSocket(socketPath); err != nil {
		return err
	}
	ln, err := net.Listen("unix", socketPath)
	if err != nil {
		return fmt.Errorf("listen %s: %w", socketPath, err)
	}
	if err := os.Chmod(socketPath, 0o600); err != nil {
		_ = ln.Close()
		return err
	}
	return s.serveListener(ctx, ln, socketPath)
}

func (s *Server) ServeTCP(ctx context.Context, addr string) error {
	ln, err := net.Listen("tcp", addr)
	if err != nil {
		return err
	}
	return s.serveListener(ctx, ln, "")
}

func (s *Server) serveListener(ctx context.Context, ln net.Listener, socketPath string) error {
	srv := &http.Server{Handler: s.mux}
	errCh := make(chan error, 1)
	go func() { errCh <- srv.Serve(ln) }()

	select {
	case <-ctx.Done():
		shutCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = srv.Shutdown(shutCtx)
		if socketPath != "" {
			_ = os.Remove(socketPath)
		}
		return nil
	case err := <-errCh:
		if socketPath != "" {
			_ = os.Remove(socketPath)
		}
		return err
	}
}

func removeStaleSocket(path string) error {
	if _, err := os.Stat(path); err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}
	conn, err := net.DialTimeout("unix", path, 200*time.Millisecond)
	if err == nil {
		conn.Close()
		return fmt.Errorf("a pando daemon is already running on %s", path)
	}
	return os.Remove(path)
}
