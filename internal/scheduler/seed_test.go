package scheduler

import (
	"testing"

	"github.com/strausslabs/pando/internal/resource"
)

func TestPhaseTerminal(t *testing.T) {
	terminal := []Phase{PhaseDone, PhaseFailed, PhaseSkipped, PhaseBlocked, PhaseStopped}
	for _, p := range terminal {
		if !p.Terminal() {
			t.Errorf("%s should be terminal", p)
		}
	}
	for _, p := range []Phase{PhasePending, PhaseStarting, PhaseHealthy, PhaseRunning} {
		if p.Terminal() {
			t.Errorf("%s should not be terminal", p)
		}
	}
}

func TestSeedRestoresPhases(t *testing.T) {
	g := graph(t, svc("db"), task("migrate", "db"))
	fe := newFakeExec()
	s := New(g, Options{Executors: map[resource.Kind]Executor{
		resource.KindLocal: fe, resource.KindTask: fe,
	}})
	s.Seed(map[string]Phase{"db": PhaseHealthy, "migrate": PhaseDone})
	if s.Phase("db") != PhaseHealthy {
		t.Errorf("seeded db phase = %s, want healthy", s.Phase("db"))
	}
	if s.Phase("migrate") != PhaseDone {
		t.Errorf("seeded migrate phase = %s, want done", s.Phase("migrate"))
	}
}

func TestPhaseUnknownResource(t *testing.T) {
	g := graph(t, svc("db"))
	fe := newFakeExec()
	s := New(g, Options{Executors: map[resource.Kind]Executor{resource.KindLocal: fe}})
	if got := s.Phase("ghost"); got != PhasePending {
		t.Errorf("unknown resource phase = %q, want pending", got)
	}
}
