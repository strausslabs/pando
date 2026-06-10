package resource

import (
	"time"
)

type Kind string

const (
	KindCompose Kind = "compose"
	KindLocal   Kind = "local"
	KindTask    Kind = "task"
)

type RunPolicy string

const (
	RunOnce     RunPolicy = "once"
	RunAlways   RunPolicy = "always"
	RunOnChange RunPolicy = "onChange"
	RunManual   RunPolicy = "manual"
)

type ProbeKind string

const (
	ProbeNone    ProbeKind = ""
	ProbeHTTPGet ProbeKind = "httpGet"
	ProbeTCP     ProbeKind = "tcp"
	ProbeLog     ProbeKind = "logMatch"
	ProbeExit0   ProbeKind = "exit0"
)

type Probe struct {
	Kind     ProbeKind     `json:"kind"`
	Target   string        `json:"target,omitempty"`
	Pattern  string        `json:"pattern,omitempty"`
	Timeout  time.Duration `json:"timeout,omitempty"`
	Interval time.Duration `json:"interval,omitempty"`
}

type BuildSecret struct {
	ID  string `json:"id" validate:"required"`
	Src string `json:"src" validate:"required"`
}

type Build struct {
	Context    string            `json:"context" validate:"required"`
	Dockerfile string            `json:"dockerfile,omitempty"`
	Args       map[string]string `json:"args,omitempty"`
	Target     string            `json:"target,omitempty"`
	Secrets    []BuildSecret     `json:"secrets,omitempty" validate:"omitempty,dive"`
}

type ComposeSpec struct {
	Image     string            `json:"image,omitempty"`
	Ports     []string          `json:"ports,omitempty"`
	Env       map[string]string `json:"env,omitempty"`
	EnvFile   []string          `json:"envFile,omitempty"`
	DependsOn []string          `json:"dependsOn,omitempty"`
	Volumes   []string          `json:"volumes,omitempty"`
	Command   []string          `json:"command,omitempty"`
	// Memory is a hard container memory limit in bytes (the DSL converts "256m"
	// and friends). Zero means unbounded.
	Memory int64 `json:"memory,omitempty" validate:"omitempty,min=0"`
	// CPUs caps CPU as a fraction of cores (1.5 = one and a half cores); the
	// backend converts to NanoCPUs. Zero means uncapped.
	CPUs float64 `json:"cpus,omitempty" validate:"omitempty,min=0"`
	// PidsLimit caps the number of processes. Zero means unlimited.
	PidsLimit int64 `json:"pidsLimit,omitempty" validate:"omitempty,min=0"`
	// Restart is the container restart policy: "", "no", "on-failure",
	// "always", or "unless-stopped".
	Restart     string       `json:"restart,omitempty" validate:"omitempty,oneof=no on-failure always unless-stopped"`
	Healthcheck *Healthcheck `json:"healthcheck,omitempty" validate:"omitempty"`
}

// Healthcheck is the Docker-native container healthcheck, distinct from Pando's
// readyWhen probe. Test is the command (Docker CMD-SHELL form when one element,
// CMD form when many).
type Healthcheck struct {
	Test     []string      `json:"test" validate:"required"`
	Interval time.Duration `json:"interval,omitempty"`
	Timeout  time.Duration `json:"timeout,omitempty"`
	Retries  int           `json:"retries,omitempty"`
}

type LocalSpec struct {
	Cmd   string            `json:"cmd" validate:"required"`
	Cwd   string            `json:"cwd,omitempty"`
	Env   map[string]string `json:"env,omitempty"`
	Watch []string          `json:"watch,omitempty"`
}

type TaskSpec struct {
	Cmd string            `json:"cmd" validate:"required"`
	Cwd string            `json:"cwd,omitempty"`
	Env map[string]string `json:"env,omitempty"`
}

type SyncRule struct {
	Local     string `json:"local" validate:"required"`
	Container string `json:"container" validate:"required"`
}

type LiveUpdateStep struct {
	Sync    *SyncRule `json:"sync,omitempty" validate:"omitempty"`
	Run     string    `json:"run,omitempty"`
	Trigger []string  `json:"trigger,omitempty"`
	Restart bool      `json:"restart,omitempty"`
}

type Hooks struct {
	PostStart string `json:"postStart,omitempty"`
	PreStop   string `json:"preStop,omitempty"`
}

type Resource struct {
	Name       string           `json:"name" validate:"required,hostname_rfc1123"`
	Kind       Kind             `json:"kind" validate:"required,oneof=compose local task"`
	Deps       []string         `json:"deps,omitempty"`
	RunWhen    RunPolicy        `json:"runWhen,omitempty" validate:"omitempty,oneof=once always onChange manual"`
	OnChange   []string         `json:"onChange,omitempty" validate:"required_if=RunWhen onChange"`
	// Every > 0 re-runs the resource on this interval after its first run (e.g.
	// a 30m sync task).
	Every      time.Duration    `json:"every,omitempty" validate:"omitempty,min=0"`
	// Preview marks a resource whose port serves a web UI; the dashboard renders
	// a live iframe of it instead of its logs.
	Preview    bool             `json:"preview,omitempty"`
	Ready      Probe            `json:"ready,omitempty"`
	Build      *Build           `json:"build,omitempty" validate:"omitempty"`
	Compose    *ComposeSpec     `json:"compose,omitempty" validate:"omitempty"`
	Local      *LocalSpec       `json:"local,omitempty" validate:"omitempty"`
	Task       *TaskSpec        `json:"task,omitempty" validate:"omitempty"`
	LiveUpdate []LiveUpdateStep `json:"liveUpdate,omitempty" validate:"omitempty,dive"`
	Hooks      Hooks            `json:"hooks,omitempty"`
}

type Stack struct {
	Name      string      `json:"name" validate:"required"`
	Resources []*Resource `json:"resources" validate:"dive"`
}

func (r *Resource) DefaultRunPolicy() RunPolicy {
	if r.RunWhen != "" {
		return r.RunWhen
	}
	// Periodic resources re-run every tick, so they must not record "ran once".
	if r.IsPeriodic() {
		return RunAlways
	}
	if r.Kind == KindTask {
		return RunOnce
	}
	return RunAlways
}

// IsPeriodic reports whether the resource re-runs on a fixed interval.
func (r *Resource) IsPeriodic() bool { return r.Every > 0 }

func (r *Resource) allDeps() []string {
	deps := append([]string(nil), r.Deps...)
	if r.Compose != nil {
		deps = append(deps, r.Compose.DependsOn...)
	}
	return dedupe(deps)
}

func (s *Stack) Get(name string) (*Resource, bool) {
	for _, r := range s.Resources {
		if r.Name == name {
			return r, true
		}
	}
	return nil, false
}

func dedupe(in []string) []string {
	if len(in) < 2 {
		return in
	}
	seen := make(map[string]bool, len(in))
	out := in[:0]
	for _, v := range in {
		if seen[v] {
			continue
		}
		seen[v] = true
		out = append(out, v)
	}
	return out
}
