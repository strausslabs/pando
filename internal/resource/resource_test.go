package resource

import (
	"testing"
	"time"
)

func TestValidateRequiresName(t *testing.T) {
	r := &Resource{Kind: KindTask, Task: &TaskSpec{Cmd: "x"}}
	if err := r.Validate(); err == nil {
		t.Fatal("expected error for empty name")
	}
}

func TestValidateKindSpecs(t *testing.T) {
	cases := []struct {
		name    string
		res     *Resource
		wantErr bool
	}{
		{"local needs cmd", &Resource{Name: "a", Kind: KindLocal, Local: &LocalSpec{}}, true},
		{"local ok", &Resource{Name: "a", Kind: KindLocal, Local: &LocalSpec{Cmd: "bun dev"}}, false},
		{"task needs cmd", &Resource{Name: "a", Kind: KindTask, Task: &TaskSpec{}}, true},
		{"task ok", &Resource{Name: "a", Kind: KindTask, Task: &TaskSpec{Cmd: "migrate"}}, false},
		{"compose needs spec or build", &Resource{Name: "a", Kind: KindCompose}, true},
		{"compose with image", &Resource{Name: "a", Kind: KindCompose, Compose: &ComposeSpec{Image: "x"}}, false},
		{"compose with build", &Resource{Name: "a", Kind: KindCompose, Build: &Build{Context: "."}}, false},
		{"unknown kind", &Resource{Name: "a", Kind: "weird"}, true},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			err := c.res.Validate()
			if (err != nil) != c.wantErr {
				t.Errorf("got err=%v, wantErr=%v", err, c.wantErr)
			}
		})
	}
}

func TestValidateOnChangeNeedsPaths(t *testing.T) {
	r := &Resource{Name: "a", Kind: KindTask, Task: &TaskSpec{Cmd: "x"}, RunWhen: RunOnChange}
	if err := r.Validate(); err == nil {
		t.Fatal("runWhen=onChange without paths must error")
	}
	r.OnChange = []string{"./migrations"}
	if err := r.Validate(); err != nil {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestDefaultRunPolicy(t *testing.T) {
	task := &Resource{Name: "t", Kind: KindTask, Task: &TaskSpec{Cmd: "x"}}
	if task.DefaultRunPolicy() != RunOnce {
		t.Errorf("task default should be once, got %s", task.DefaultRunPolicy())
	}
	svc := &Resource{Name: "s", Kind: KindLocal, Local: &LocalSpec{Cmd: "x"}}
	if svc.DefaultRunPolicy() != RunAlways {
		t.Errorf("service default should be always, got %s", svc.DefaultRunPolicy())
	}
	explicit := &Resource{Name: "e", Kind: KindTask, Task: &TaskSpec{Cmd: "x"}, RunWhen: RunManual}
	if explicit.DefaultRunPolicy() != RunManual {
		t.Errorf("explicit policy must win, got %s", explicit.DefaultRunPolicy())
	}
}

func TestIsPeriodic(t *testing.T) {
	cases := []struct {
		name  string
		every time.Duration
		want  bool
	}{
		{"zero is not periodic", 0, false},
		{"positive is periodic", 30 * time.Minute, true},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			r := &Resource{Name: "r", Kind: KindTask, Task: &TaskSpec{Cmd: "x"}, Every: c.every}
			if got := r.IsPeriodic(); got != c.want {
				t.Errorf("IsPeriodic()=%v, want %v", got, c.want)
			}
		})
	}
}

func TestDefaultRunPolicyPeriodicTaskIsAlways(t *testing.T) {
	// A periodic task must re-run every tick, so its default must be RunAlways,
	// not RunOnce — otherwise the first run would suppress all later ticks.
	r := &Resource{Name: "sync", Kind: KindTask, Task: &TaskSpec{Cmd: "x"}, Every: 30 * time.Minute}
	if got := r.DefaultRunPolicy(); got != RunAlways {
		t.Errorf("periodic task default = %s, want %s", got, RunAlways)
	}
}

func TestDefaultRunPolicyByKind(t *testing.T) {
	cases := []struct {
		name string
		kind Kind
		want RunPolicy
	}{
		{"non-periodic task is once", KindTask, RunOnce},
		{"local is always", KindLocal, RunAlways},
		{"compose is always", KindCompose, RunAlways},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			r := &Resource{Name: "r", Kind: c.kind}
			if got := r.DefaultRunPolicy(); got != c.want {
				t.Errorf("DefaultRunPolicy()=%s, want %s", got, c.want)
			}
		})
	}
}

func TestDefaultRunPolicyExplicitOverridesPeriodic(t *testing.T) {
	// An explicit RunWhen wins over every default, including the periodic rule.
	r := &Resource{Name: "r", Kind: KindTask, Task: &TaskSpec{Cmd: "x"}, Every: 30 * time.Minute, RunWhen: RunManual}
	if got := r.DefaultRunPolicy(); got != RunManual {
		t.Errorf("explicit runWhen must win even when periodic, got %s", got)
	}
}

func TestStackValidateDuplicateNames(t *testing.T) {
	s := &Stack{Name: "s", Resources: []*Resource{
		{Name: "dup", Kind: KindTask, Task: &TaskSpec{Cmd: "x"}},
		{Name: "dup", Kind: KindTask, Task: &TaskSpec{Cmd: "y"}},
	}}
	if err := s.Validate(); err == nil {
		t.Fatal("expected duplicate name error")
	}
}

func TestStackValidateUnknownDep(t *testing.T) {
	s := &Stack{Name: "s", Resources: []*Resource{
		{Name: "a", Kind: KindTask, Task: &TaskSpec{Cmd: "x"}, Deps: []string{"ghost"}},
	}}
	if err := s.Validate(); err == nil {
		t.Fatal("expected unknown dep error")
	}
}

func TestStackGet(t *testing.T) {
	s := &Stack{Name: "s", Resources: []*Resource{
		{Name: "a", Kind: KindTask, Task: &TaskSpec{Cmd: "x"}},
	}}
	if _, ok := s.Get("a"); !ok {
		t.Error("should find a")
	}
	if _, ok := s.Get("missing"); ok {
		t.Error("should not find missing")
	}
}

func TestDedupe(t *testing.T) {
	tests := []struct {
		name string
		in   []string
		want []string
	}{
		{"nil", nil, nil},
		{"single", []string{"a"}, []string{"a"}},
		{"duplicates", []string{"a", "a", "b", "a"}, []string{"a", "b"}},
		{"already unique", []string{"a", "b", "c"}, []string{"a", "b", "c"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := dedupe(tt.in)
			if len(got) != len(tt.want) {
				t.Fatalf("dedupe(%v) = %v, want %v", tt.in, got, tt.want)
			}
			for i := range got {
				if got[i] != tt.want[i] {
					t.Fatalf("dedupe(%v) = %v, want %v", tt.in, got, tt.want)
				}
			}
		})
	}
}

func TestAllDeps(t *testing.T) {
	plain := &Resource{Deps: []string{"a", "b"}}
	if got := plain.AllDeps(); len(got) != 2 || got[0] != "a" || got[1] != "b" {
		t.Errorf("AllDeps without compose = %v, want [a b]", got)
	}

	withCompose := &Resource{
		Deps:    []string{"a", "b"},
		Compose: &ComposeSpec{DependsOn: []string{"b", "c"}},
	}
	got := withCompose.AllDeps()
	if len(got) != 3 || got[0] != "a" || got[1] != "b" || got[2] != "c" {
		t.Errorf("AllDeps merges+dedupes = %v, want [a b c]", got)
	}
}
