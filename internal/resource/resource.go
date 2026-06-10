package resource

import (
	"fmt"
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

type Build struct {
	Context    string            `json:"context"`
	Dockerfile string            `json:"dockerfile,omitempty"`
	Args       map[string]string `json:"args,omitempty"`
	Target     string            `json:"target,omitempty"`
}

type ComposeSpec struct {
	Image     string            `json:"image,omitempty"`
	Ports     []string          `json:"ports,omitempty"`
	Env       map[string]string `json:"env,omitempty"`
	DependsOn []string          `json:"dependsOn,omitempty"`
	Volumes   []string          `json:"volumes,omitempty"`
	Command   []string          `json:"command,omitempty"`
}

type LocalSpec struct {
	Cmd   string            `json:"cmd"`
	Cwd   string            `json:"cwd,omitempty"`
	Env   map[string]string `json:"env,omitempty"`
	Watch []string          `json:"watch,omitempty"`
}

type TaskSpec struct {
	Cmd string            `json:"cmd"`
	Cwd string            `json:"cwd,omitempty"`
	Env map[string]string `json:"env,omitempty"`
}

type SyncRule struct {
	Local     string `json:"local"`
	Container string `json:"container"`
}

type LiveUpdateStep struct {
	Sync    *SyncRule `json:"sync,omitempty"`
	Run     string    `json:"run,omitempty"`
	Trigger []string  `json:"trigger,omitempty"`
	Restart bool      `json:"restart,omitempty"`
}

type Hooks struct {
	PostStart string `json:"postStart,omitempty"`
	PreStop   string `json:"preStop,omitempty"`
}

type Resource struct {
	Name       string           `json:"name"`
	Kind       Kind             `json:"kind"`
	Deps       []string         `json:"deps,omitempty"`
	RunWhen    RunPolicy        `json:"runWhen,omitempty"`
	OnChange   []string         `json:"onChange,omitempty"`
	Ready      Probe            `json:"ready,omitempty"`
	Build      *Build           `json:"build,omitempty"`
	Compose    *ComposeSpec     `json:"compose,omitempty"`
	Local      *LocalSpec       `json:"local,omitempty"`
	Task       *TaskSpec        `json:"task,omitempty"`
	LiveUpdate []LiveUpdateStep `json:"liveUpdate,omitempty"`
	Hooks      Hooks            `json:"hooks,omitempty"`
}

type Stack struct {
	Name      string      `json:"name"`
	Resources []*Resource `json:"resources"`
}

func (r *Resource) Validate() error {
	if r.Name == "" {
		return fmt.Errorf("resource has empty name")
	}
	switch r.Kind {
	case KindCompose:
		if r.Compose == nil && r.Build == nil {
			return fmt.Errorf("resource %q: compose kind needs compose or build spec", r.Name)
		}
	case KindLocal:
		if r.Local == nil || r.Local.Cmd == "" {
			return fmt.Errorf("resource %q: local kind needs local.cmd", r.Name)
		}
	case KindTask:
		if r.Task == nil || r.Task.Cmd == "" {
			return fmt.Errorf("resource %q: task kind needs task.cmd", r.Name)
		}
	default:
		return fmt.Errorf("resource %q: unknown kind %q", r.Name, r.Kind)
	}
	if r.RunWhen == RunOnChange && len(r.OnChange) == 0 {
		return fmt.Errorf("resource %q: runWhen=onChange needs onChange paths", r.Name)
	}
	return nil
}

func (r *Resource) DefaultRunPolicy() RunPolicy {
	if r.RunWhen != "" {
		return r.RunWhen
	}
	if r.Kind == KindTask {
		return RunOnce
	}
	return RunAlways
}

func (s *Stack) Validate() error {
	if s.Name == "" {
		return fmt.Errorf("stack has empty name")
	}
	seen := make(map[string]bool, len(s.Resources))
	for _, r := range s.Resources {
		if seen[r.Name] {
			return fmt.Errorf("duplicate resource name %q", r.Name)
		}
		seen[r.Name] = true
		if err := r.Validate(); err != nil {
			return err
		}
	}
	for _, r := range s.Resources {
		for _, d := range r.allDeps() {
			if !seen[d] {
				return fmt.Errorf("resource %q depends on unknown resource %q", r.Name, d)
			}
		}
	}
	return nil
}

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
