package probe

import (
	"context"
	"net"
	"net/http"
	"net/http/httptest"
	"strconv"
	"sync/atomic"
	"testing"
	"time"

	"github.com/guyStrauss/pando/internal/interp"
	"github.com/guyStrauss/pando/internal/resource"
)

func fastProbe(kind resource.ProbeKind, target string) resource.Probe {
	return resource.Probe{Kind: kind, Target: target, Timeout: 2 * time.Second, Interval: 10 * time.Millisecond}
}

func TestNoneProbePassesImmediately(t *testing.T) {
	if err := Wait(context.Background(), resource.Probe{Kind: resource.ProbeNone}, Options{}); err != nil {
		t.Errorf("none probe should pass, got %v", err)
	}
}

func TestHTTPGetSucceeds(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(200)
	}))
	defer srv.Close()
	p := fastProbe(resource.ProbeHTTPGet, srv.URL)
	if err := Wait(context.Background(), p, Options{Scope: interp.Scope{}}); err != nil {
		t.Errorf("expected success, got %v", err)
	}
}

func TestHTTPGetEventuallyReady(t *testing.T) {
	var ready int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if atomic.LoadInt32(&ready) == 0 {
			w.WriteHeader(503)
			return
		}
		w.WriteHeader(200)
	}))
	defer srv.Close()
	go func() {
		time.Sleep(100 * time.Millisecond)
		atomic.StoreInt32(&ready, 1)
	}()
	p := fastProbe(resource.ProbeHTTPGet, srv.URL)
	if err := Wait(context.Background(), p, Options{Scope: interp.Scope{}}); err != nil {
		t.Errorf("should pass once server becomes ready, got %v", err)
	}
}

func TestHTTPGetTimeout(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(500)
	}))
	defer srv.Close()
	p := resource.Probe{Kind: resource.ProbeHTTPGet, Target: srv.URL, Timeout: 150 * time.Millisecond, Interval: 10 * time.Millisecond}
	if err := Wait(context.Background(), p, Options{Scope: interp.Scope{}}); err == nil {
		t.Error("expected timeout error for always-500 endpoint")
	}
}

func TestHTTPGetInterpolatesPort(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(200)
	}))
	defer srv.Close()
	_, portStr, _ := net.SplitHostPort(srv.Listener.Addr().String())
	port, _ := strconv.Atoi(portStr)
	scope := interp.Scope{Ports: map[string]int{"api": port}}
	p := fastProbe(resource.ProbeHTTPGet, "http://127.0.0.1:$PORT_api/")
	if err := Wait(context.Background(), p, Options{Scope: scope}); err != nil {
		t.Errorf("interpolated probe should pass, got %v", err)
	}
}

func TestTCPSucceeds(t *testing.T) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer ln.Close()
	go func() {
		for {
			c, err := ln.Accept()
			if err != nil {
				return
			}
			c.Close()
		}
	}()
	p := fastProbe(resource.ProbeTCP, ln.Addr().String())
	if err := Wait(context.Background(), p, Options{Scope: interp.Scope{}}); err != nil {
		t.Errorf("tcp probe should pass, got %v", err)
	}
}

func TestTCPTimeout(t *testing.T) {
	p := resource.Probe{Kind: resource.ProbeTCP, Target: "127.0.0.1:1", Timeout: 150 * time.Millisecond, Interval: 20 * time.Millisecond}
	if err := Wait(context.Background(), p, Options{Scope: interp.Scope{}}); err == nil {
		t.Error("expected timeout dialing closed port")
	}
}

type fakeLogs struct{ lines []string }

func (f fakeLogs) Text(string, string) []string { return f.lines }

func TestLogMatchSucceeds(t *testing.T) {
	logs := fakeLogs{lines: []string{"booting", "Server listening on 8000", "ready"}}
	p := fastProbe(resource.ProbeLog, "")
	p.Pattern = "listening on"
	err := Wait(context.Background(), p, Options{Logs: logs, Worktree: "main", Resource: "api"})
	if err != nil {
		t.Errorf("logMatch should pass, got %v", err)
	}
}

func TestLogMatchTimeout(t *testing.T) {
	logs := fakeLogs{lines: []string{"still starting"}}
	p := resource.Probe{Kind: resource.ProbeLog, Pattern: "ready", Timeout: 100 * time.Millisecond, Interval: 20 * time.Millisecond}
	if err := Wait(context.Background(), p, Options{Logs: logs}); err == nil {
		t.Error("expected timeout when pattern never appears")
	}
}

func TestLogMatchBadPattern(t *testing.T) {
	p := resource.Probe{Kind: resource.ProbeLog, Pattern: "("}
	if err := Wait(context.Background(), p, Options{Logs: fakeLogs{}}); err == nil {
		t.Error("invalid regex should error")
	}
}

func TestExit0PassesImmediately(t *testing.T) {
	if err := Wait(context.Background(), resource.Probe{Kind: resource.ProbeExit0}, Options{}); err != nil {
		t.Errorf("exit0 should pass immediately, got %v", err)
	}
}

func TestContextCancelStopsProbe(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	p := fastProbe(resource.ProbeTCP, "127.0.0.1:1")
	if err := Wait(ctx, p, Options{Scope: interp.Scope{}}); err == nil {
		t.Error("cancelled context should abort probe")
	}
}
