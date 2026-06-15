package daemon

import (
	"context"
	"net"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/guyStrauss/pando/internal/client"
)

func shortSock(t *testing.T) string {
	t.Helper()
	dir, err := os.MkdirTemp("", "pd")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(dir) })
	return filepath.Join(dir, "s")
}

func waitHealthy(t *testing.T, sock string) {
	t.Helper()
	for i := 0; i < 100; i++ {
		ctx, cancel := context.WithTimeout(context.Background(), 200*time.Millisecond)
		err := client.New(sock).Health(ctx)
		cancel()
		if err == nil {
			return
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatal("daemon never became healthy")
}

func TestServeAndShutdown(t *testing.T) {
	s, _ := testServer()
	sock := shortSock(t)
	ctx, cancel := context.WithCancel(context.Background())

	errCh := make(chan error, 1)
	go func() { errCh <- s.Serve(ctx, sock) }()

	waitHealthy(t, sock)
	cancel()

	select {
	case err := <-errCh:
		if err != nil {
			t.Errorf("Serve returned %v, want nil on ctx cancel", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("Serve did not return after cancel")
	}
	if _, err := os.Stat(sock); !os.IsNotExist(err) {
		t.Errorf("socket should be removed on shutdown, stat err = %v", err)
	}
}

func TestServeStaleLiveSocketRefused(t *testing.T) {
	s, _ := testServer()
	sock := shortSock(t)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	go func() { _ = s.Serve(ctx, sock) }()
	waitHealthy(t, sock)

	s2, _ := testServer()
	err := s2.Serve(ctx, sock)
	if err == nil || !contains(err.Error(), "already running") {
		t.Errorf("second Serve on live socket should refuse, got %v", err)
	}
}

func TestRemoveStaleDeadSocket(t *testing.T) {
	sock := shortSock(t)
	ln, err := net.Listen("unix", sock)
	if err != nil {
		t.Fatal(err)
	}
	_ = ln.Close() // leaves the socket file but nothing listening

	if err := removeStaleSocket(sock); err != nil {
		t.Errorf("dead socket should be removed, got %v", err)
	}
	if _, err := os.Stat(sock); !os.IsNotExist(err) {
		t.Errorf("dead socket file should be gone, stat err = %v", err)
	}
}

func TestRemoveStaleNonexistent(t *testing.T) {
	if err := removeStaleSocket(filepath.Join(t.TempDir(), "nope")); err != nil {
		t.Errorf("nonexistent path should be nil, got %v", err)
	}
}

func TestServeTCPBadAddr(t *testing.T) {
	s, _ := testServer()
	if err := s.ServeTCP(context.Background(), "256.256.256.256:99999"); err == nil {
		t.Error("invalid tcp addr should error")
	}
}

func TestServeTCPServesAndShuts(t *testing.T) {
	s, _ := testServer()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	addr := ln.Addr().String()
	_ = ln.Close()

	ctx, cancel := context.WithCancel(context.Background())
	errCh := make(chan error, 1)
	go func() { errCh <- s.ServeTCP(ctx, addr) }()

	for i := 0; i < 100; i++ {
		c, err := net.DialTimeout("tcp", addr, 100*time.Millisecond)
		if err == nil {
			_ = c.Close()
			break
		}
		time.Sleep(20 * time.Millisecond)
	}
	cancel()
	select {
	case err := <-errCh:
		if err != nil {
			t.Errorf("ServeTCP returned %v, want nil on cancel", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("ServeTCP did not return after cancel")
	}
}

func contains(s, sub string) bool {
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return true
		}
	}
	return false
}
