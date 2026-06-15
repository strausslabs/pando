package config

import (
	"context"
	"strings"
	"testing"

	"github.com/guyStrauss/pando/internal/resource"
)

func loadSrc(t *testing.T, src string) (*resource.Stack, error) {
	t.Helper()
	raw, err := eval(context.Background(), "test.star", []byte(src))
	if err != nil {
		return nil, err
	}
	return decode(raw)
}

func mustLoadSrc(t *testing.T, src string) *resource.Stack {
	t.Helper()
	st, err := loadSrc(t, src)
	if err != nil {
		t.Fatalf("loadSrc: %v", err)
	}
	return st
}

func TestProbeDurationForms(t *testing.T) {
	st := mustLoadSrc(t, `
define_stack(
    name = "d",
    services = {
        "p": service(
            task = task(cmd = "true"),
            ready = http_get("http://x", interval = "500ms", timeout = "2s"),
        ),
    },
)
`)
	r := st.Resources[0]
	if r.Ready.Interval.Milliseconds() != 500 {
		t.Errorf("interval = %v, want 500ms", r.Ready.Interval)
	}
	if r.Ready.Timeout.Seconds() != 2 {
		t.Errorf("timeout = %v, want 2s", r.Ready.Timeout)
	}
}

func TestDurationBuiltinAcceptsIntNanos(t *testing.T) {
	st := mustLoadSrc(t, `
define_stack(
    name = "d",
    services = {"p": service(task = task(cmd = "true"), every = duration(60000000000))},
)
`)
	if st.Resources[0].Every.Minutes() != 1 {
		t.Errorf("int-ns duration not honored: %v", st.Resources[0].Every)
	}
}

func TestBadDurationErrors(t *testing.T) {
	_, err := loadSrc(t, `
define_stack(
    name = "d",
    services = {"p": service(task = task(cmd = "true"), every = duration("soon"))},
)
`)
	if err == nil || !strings.Contains(err.Error(), "duration") {
		t.Errorf("bad duration should error, got %v", err)
	}
}

func TestBytesBuiltinForms(t *testing.T) {
	st := mustLoadSrc(t, `
define_stack(
    name = "b",
    services = {"c": service(compose = compose(image = "alpine", memory = bytes("256m")))},
)
`)
	if got := st.Resources[0].Compose.Memory; got != 256*(1<<20) {
		t.Errorf("memory = %d, want %d", got, 256*(1<<20))
	}
}

func TestBytesBuiltinPlainAndKilo(t *testing.T) {
	st := mustLoadSrc(t, `
define_stack(
    name = "b",
    services = {"c": service(compose = compose(image = "alpine", memory = bytes("2k")))},
)
`)
	if got := st.Resources[0].Compose.Memory; got != 2*1024 {
		t.Errorf("memory = %d, want 2048", got)
	}
}

func TestBadBytesErrors(t *testing.T) {
	_, err := loadSrc(t, `
define_stack(
    name = "b",
    services = {"c": service(compose = compose(image = "alpine", memory = bytes("lots")))},
)
`)
	if err == nil || !strings.Contains(err.Error(), "size") {
		t.Errorf("bad size should error, got %v", err)
	}
}

func TestOnChangeIgnoreParses(t *testing.T) {
	st := mustLoadSrc(t, `
define_stack(
    name = "s",
    services = {
        "build": service(
            task = task(cmd = "go build ./..."),
            runWhen = "onChange",
            onChange = ["**/*.go"],
            ignore = ["**/*_test.go", "vendor"],
        ),
    },
)
`)
	r := st.Resources[0]
	if len(r.OnChange) != 1 || len(r.Ignore) != 2 || r.Ignore[0] != "**/*_test.go" {
		t.Errorf("onChange/ignore not parsed: onChange=%v ignore=%v", r.OnChange, r.Ignore)
	}
}

func TestDepsStringList(t *testing.T) {
	st := mustLoadSrc(t, `
define_stack(
    name = "s",
    services = {
        "a": service(task = task(cmd = "true")),
        "b": service(task = task(cmd = "true"), deps = ["a"]),
    },
)
`)
	var b *resource.Resource
	for _, r := range st.Resources {
		if r.Name == "b" {
			b = r
		}
	}
	if b == nil || len(b.Deps) != 1 || b.Deps[0] != "a" {
		t.Errorf("deps not parsed as string list: %+v", b)
	}
}

func TestLiveUpdateStepsAndTriggerList(t *testing.T) {
	st := mustLoadSrc(t, `
define_stack(
    name = "lu",
    services = {
        "api": service(
            compose = compose(image = "alpine"),
            liveUpdate = [
                sync("./src", "/app/src"),
                run("make", trigger = ["a.go", "b.go"]),
                restart_container(),
            ],
        ),
    },
)
`)
	steps := st.Resources[0].LiveUpdate
	if len(steps) != 3 {
		t.Fatalf("want 3 live-update steps, got %d", len(steps))
	}
	if len(steps[1].Trigger) != 2 {
		t.Errorf("trigger list not parsed: %+v", steps[1].Trigger)
	}
	if !steps[2].RestartContainer {
		t.Errorf("restart_container() should set RestartContainer: %+v", steps[2])
	}
}

func TestRunTriggerWrongTypeErrors(t *testing.T) {
	_, err := loadSrc(t, `
define_stack(
    name = "lu",
    services = {
        "api": service(
            compose = compose(image = "alpine"),
            liveUpdate = [run("make", trigger = 42)],
        ),
    },
)
`)
	if err == nil || !strings.Contains(err.Error(), "trigger") {
		t.Errorf("non-string/list trigger should error, got %v", err)
	}
}

func TestMissingDefineStackErrors(t *testing.T) {
	_, err := eval(context.Background(), "t.star", []byte(`x = 1`))
	if err == nil || !strings.Contains(err.Error(), "define_stack") {
		t.Errorf("config without define_stack should error, got %v", err)
	}
}

func TestStarlarkSyntaxErrorReported(t *testing.T) {
	_, err := eval(context.Background(), "t.star", []byte(`define_stack(`))
	if err == nil || !strings.Contains(err.Error(), "evaluate config") {
		t.Errorf("syntax error should be reported, got %v", err)
	}
}
