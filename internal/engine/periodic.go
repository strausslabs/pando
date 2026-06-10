package engine

import (
	"context"
	"fmt"
	"time"

	"github.com/guyStrauss/pando/internal/logbuf"
)

// startPeriodic restarts the tickers for a stack's periodic resources, safe to
// call after Up and Reload. The first tick fires one interval out: the initial
// run already happened during Up.
func (e *Engine) startPeriodic(as *activeStack) {
	as.mu.Lock()
	if as.periodicCancel != nil {
		as.periodicCancel()
		as.periodicCancel = nil
	}
	var periodic []string
	for _, r := range as.stack.Resources {
		if r.IsPeriodic() {
			periodic = append(periodic, r.Name)
		}
	}
	if len(periodic) == 0 {
		as.mu.Unlock()
		return
	}
	ctx, cancel := context.WithCancel(context.Background())
	as.periodicCancel = cancel
	as.mu.Unlock()

	for _, name := range periodic {
		r, _ := as.stack.Get(name)
		as.mu.Lock()
		as.nextRun[name] = e.cfg.Clock().Add(r.Every)
		as.mu.Unlock()
		go e.periodicLoop(ctx, as, name, r.Every)
	}
}

func (e *Engine) periodicLoop(ctx context.Context, as *activeStack, name string, every time.Duration) {
	ticker := time.NewTicker(every)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			if err := as.sched.UpSubset(ctx, name); err != nil && e.cfg.Logs != nil {
				e.cfg.Logs.Append(as.env.Worktree, name, logbuf.System,
					fmt.Sprintf("periodic run failed: %v", err),
					func() logbuf.Line { return logbuf.Line{Time: e.cfg.Clock()} })
			}
			as.mu.Lock()
			as.nextRun[name] = e.cfg.Clock().Add(every)
			as.mu.Unlock()
		}
	}
}

func (as *activeStack) stopPeriodic() {
	as.mu.Lock()
	if as.periodicCancel != nil {
		as.periodicCancel()
		as.periodicCancel = nil
	}
	as.mu.Unlock()
}
