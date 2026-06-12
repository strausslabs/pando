package probe

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"regexp"
	"time"

	"github.com/guyStrauss/pando/internal/interp"
	"github.com/guyStrauss/pando/internal/resource"
)

const (
	defaultTimeout  = 30 * time.Second
	defaultInterval = 500 * time.Millisecond
)

type LogQuerier interface {
	Text(worktree, resource string) []string
}

type Options struct {
	Scope      interp.Scope
	Worktree   string
	Resource   string
	HTTPClient *http.Client
	Logs       LogQuerier
	Dialer     func(ctx context.Context, network, addr string) (net.Conn, error)
}

func Wait(ctx context.Context, p resource.Probe, opts Options) error {
	if p.Kind == resource.ProbeNone {
		return nil
	}
	check, err := checkerFor(p, opts)
	if err != nil {
		return err
	}

	timeout := p.Timeout
	if timeout <= 0 {
		timeout = defaultTimeout
	}
	interval := p.Interval
	if interval <= 0 {
		interval = defaultInterval
	}

	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	var lastErr error
	for {
		if lastErr = check(ctx); lastErr == nil {
			return nil
		}
		select {
		case <-ctx.Done():
			return fmt.Errorf("probe %s did not pass within %s: %w", p.Kind, timeout, lastErr)
		case <-ticker.C:
		}
	}
}

type checkFunc func(ctx context.Context) error

func checkerFor(p resource.Probe, opts Options) (checkFunc, error) {
	switch p.Kind {
	case resource.ProbeHTTPGet:
		target, err := opts.Scope.String(p.Target)
		if err != nil {
			return nil, err
		}
		client := opts.HTTPClient
		if client == nil {
			client = &http.Client{Timeout: 5 * time.Second}
		}
		return func(ctx context.Context) error {
			req, err := http.NewRequestWithContext(ctx, http.MethodGet, target, nil)
			if err != nil {
				return err
			}
			resp, err := client.Do(req)
			if err != nil {
				return err
			}
			defer resp.Body.Close()
			if resp.StatusCode < 200 || resp.StatusCode >= 400 {
				return fmt.Errorf("status %d", resp.StatusCode)
			}
			return nil
		}, nil

	case resource.ProbeTCP:
		target, err := opts.Scope.String(p.Target)
		if err != nil {
			return nil, err
		}
		dial := opts.Dialer
		if dial == nil {
			d := &net.Dialer{Timeout: 5 * time.Second}
			dial = d.DialContext
		}
		return func(ctx context.Context) error {
			conn, err := dial(ctx, "tcp", target)
			if err != nil {
				return err
			}
			return conn.Close()
		}, nil

	case resource.ProbeLog:
		re, err := regexp.Compile(p.Pattern)
		if err != nil {
			return nil, fmt.Errorf("logMatch pattern: %w", err)
		}
		if opts.Logs == nil {
			return nil, fmt.Errorf("logMatch probe requires a log querier")
		}
		return func(ctx context.Context) error {
			for _, line := range opts.Logs.Text(opts.Worktree, opts.Resource) {
				if re.MatchString(line) {
					return nil
				}
			}
			return fmt.Errorf("pattern %q not seen yet", p.Pattern)
		}, nil

	case resource.ProbeExit0:
		return func(context.Context) error { return nil }, nil

	default:
		return nil, fmt.Errorf("unknown probe kind %q", p.Kind)
	}
}
