package config

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"

	"go.starlark.net/starlark"
)

var (
	durRe   = regexp.MustCompile(`^(\d+(?:\.\d+)?)(ms|s|m|h)$`)
	bytesRe = regexp.MustCompile(`^(?i)(\d+(?:\.\d+)?)\s*(b|k|kb|m|mb|g|gb)?$`)
)

type holder struct {
	stack starlark.Value
	set   bool
}

func builtins(h *holder) starlark.StringDict {
	return starlark.StringDict{
		"define_stack": starlark.NewBuiltin("define_stack", h.defineStack),
		"service":      starlark.NewBuiltin("service", makeMapBuiltin("service")),
		"cmd":          starlark.NewBuiltin("cmd", buildCmd),
		"task":         starlark.NewBuiltin("task", buildCmd),
		"compose":      starlark.NewBuiltin("compose", makeMapBuiltin("compose")),
		"build":        starlark.NewBuiltin("build", makeMapBuiltin("build")),
		"healthcheck":  starlark.NewBuiltin("healthcheck", makeMapBuiltin("healthcheck")),
		"http_get":     probeBuiltin("httpGet"),
		"tcp":          probeBuiltin("tcp"),
		"log_match":    probeBuiltin("logMatch"),
		"exit0":        probeBuiltin("exit0"),
		"sync":         starlark.NewBuiltin("sync", buildSync),
		"run":          starlark.NewBuiltin("run", buildRun),
		"restart":      starlark.NewBuiltin("restart", buildRestart),
		"duration":     starlark.NewBuiltin("duration", durationBuiltin),
		"bytes":        starlark.NewBuiltin("bytes", bytesBuiltin),
	}
}

func (h *holder) defineStack(_ *starlark.Thread, _ *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
	d, err := kwargsToDict(args, kwargs)
	if err != nil {
		return nil, err
	}
	h.stack = d
	h.set = true
	return d, nil
}

func makeMapBuiltin(name string) func(*starlark.Thread, *starlark.Builtin, starlark.Tuple, []starlark.Tuple) (starlark.Value, error) {
	return func(_ *starlark.Thread, _ *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
		if len(args) > 0 {
			return nil, fmt.Errorf("%s: only keyword arguments are allowed", name)
		}
		return kwargsToDict(nil, kwargs)
	}
}

func buildCmd(_ *starlark.Thread, b *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
	var cmd, cwd string
	var env *starlark.Dict
	var watch *starlark.List
	if err := starlark.UnpackArgs(b.Name(), args, kwargs, "cmd", &cmd, "cwd?", &cwd, "env?", &env, "watch?", &watch); err != nil {
		return nil, err
	}
	d := starlark.NewDict(4)
	_ = d.SetKey(starlark.String("cmd"), starlark.String(cmd))
	if cwd != "" {
		_ = d.SetKey(starlark.String("cwd"), starlark.String(cwd))
	}
	if env != nil {
		_ = d.SetKey(starlark.String("env"), env)
	}
	if watch != nil {
		_ = d.SetKey(starlark.String("watch"), watch)
	}
	return d, nil
}

func buildSync(_ *starlark.Thread, b *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
	var local, container string
	if err := starlark.UnpackArgs(b.Name(), args, kwargs, "local", &local, "container", &container); err != nil {
		return nil, err
	}
	inner := starlark.NewDict(2)
	_ = inner.SetKey(starlark.String("local"), starlark.String(local))
	_ = inner.SetKey(starlark.String("container"), starlark.String(container))
	step := starlark.NewDict(1)
	_ = step.SetKey(starlark.String("sync"), inner)
	return step, nil
}

func buildRun(_ *starlark.Thread, b *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
	var cmd string
	var trigger starlark.Value
	if err := starlark.UnpackArgs(b.Name(), args, kwargs, "cmd", &cmd, "trigger?", &trigger); err != nil {
		return nil, err
	}
	step := starlark.NewDict(2)
	_ = step.SetKey(starlark.String("run"), starlark.String(cmd))
	if trigger != nil {
		list, err := asStringList(trigger)
		if err != nil {
			return nil, fmt.Errorf("run: trigger: %w", err)
		}
		_ = step.SetKey(starlark.String("trigger"), list)
	}
	return step, nil
}

func buildRestart(_ *starlark.Thread, b *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
	if err := starlark.UnpackArgs(b.Name(), args, kwargs); err != nil {
		return nil, err
	}
	step := starlark.NewDict(1)
	_ = step.SetKey(starlark.String("restart"), starlark.Bool(true))
	return step, nil
}

func probeBuiltin(kind string) *starlark.Builtin {
	return starlark.NewBuiltin(kind, func(_ *starlark.Thread, b *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
		var target string
		var timeout, interval starlark.Value
		spec := []any{"timeout?", &timeout, "interval?", &interval}
		if kind != "exit0" {
			spec = append([]any{"target", &target}, spec...)
		}
		if err := starlark.UnpackArgs(b.Name(), args, kwargs, spec...); err != nil {
			return nil, err
		}
		d := starlark.NewDict(4)
		_ = d.SetKey(starlark.String("kind"), starlark.String(kind))
		switch kind {
		case "httpGet", "tcp":
			_ = d.SetKey(starlark.String("target"), starlark.String(target))
		case "logMatch":
			_ = d.SetKey(starlark.String("pattern"), starlark.String(target))
		}
		if err := putDuration(d, "timeout", timeout); err != nil {
			return nil, err
		}
		if err := putDuration(d, "interval", interval); err != nil {
			return nil, err
		}
		return d, nil
	})
}

func durationBuiltin(_ *starlark.Thread, b *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
	var v starlark.Value
	if err := starlark.UnpackArgs(b.Name(), args, kwargs, "v", &v); err != nil {
		return nil, err
	}
	ns, err := toNanos(v)
	if err != nil {
		return nil, err
	}
	return starlark.MakeInt64(ns), nil
}

func bytesBuiltin(_ *starlark.Thread, b *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
	var v starlark.Value
	if err := starlark.UnpackArgs(b.Name(), args, kwargs, "v", &v); err != nil {
		return nil, err
	}
	n, err := toBytes(v)
	if err != nil {
		return nil, err
	}
	return starlark.MakeInt64(n), nil
}

func putDuration(d *starlark.Dict, key string, v starlark.Value) error {
	if v == nil {
		return nil
	}
	ns, err := toNanos(v)
	if err != nil {
		return fmt.Errorf("%s: %w", key, err)
	}
	return d.SetKey(starlark.String(key), starlark.MakeInt64(ns))
}

func kwargsToDict(args starlark.Tuple, kwargs []starlark.Tuple) (*starlark.Dict, error) {
	if len(args) > 0 {
		return nil, fmt.Errorf("only keyword arguments are allowed")
	}
	d := starlark.NewDict(len(kwargs))
	for _, kw := range kwargs {
		if err := d.SetKey(kw[0], kw[1]); err != nil {
			return nil, err
		}
	}
	return d, nil
}

func asStringList(v starlark.Value) (*starlark.List, error) {
	if s, ok := starlark.AsString(v); ok {
		return starlark.NewList([]starlark.Value{starlark.String(s)}), nil
	}
	if l, ok := v.(*starlark.List); ok {
		return l, nil
	}
	return nil, fmt.Errorf("want string or list of strings, got %s", v.Type())
}

func toNanos(v starlark.Value) (int64, error) {
	return parseQuantity(v, durRe, map[string]float64{"ms": 1e6, "s": 1e9, "m": 60e9, "h": 3600e9}, "duration")
}

func toBytes(v starlark.Value) (int64, error) {
	return parseQuantity(v, bytesRe, map[string]float64{"": 1, "b": 1, "k": 1024, "kb": 1024, "m": 1 << 20, "mb": 1 << 20, "g": 1 << 30, "gb": 1 << 30}, "size")
}

func parseQuantity(v starlark.Value, re *regexp.Regexp, units map[string]float64, typeWord string) (int64, error) {
	if i, ok := v.(starlark.Int); ok {
		n, _ := i.Int64()
		return n, nil
	}
	s, ok := starlark.AsString(v)
	if !ok {
		unit := "ns"
		if typeWord == "size" {
			unit = "bytes"
		}
		return 0, fmt.Errorf("want %s string or int %s, got %s", typeWord, unit, v.Type())
	}
	m := re.FindStringSubmatch(strings.TrimSpace(s))
	if m == nil {
		return 0, fmt.Errorf("bad %s %q", typeWord, s)
	}
	f, _ := strconv.ParseFloat(m[1], 64)
	return int64(f * units[strings.ToLower(m[2])]), nil
}
