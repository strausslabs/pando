package config

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os"

	"github.com/guyStrauss/pando/internal/resource"
	"go.starlark.net/starlark"
	"go.starlark.net/syntax"
)

// DefaultConfigName is the Starlark config file Pando looks for in a worktree.
const DefaultConfigName = "pando.star"

type Loader struct{}

func NewLoader() (*Loader, error) { return &Loader{}, nil }

func (l *Loader) LoadFile(ctx context.Context, path string) (*resource.Stack, error) {
	src, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("config not found: %w", err)
	}
	raw, err := eval(ctx, path, src)
	if err != nil {
		return nil, err
	}
	stack, err := decode(raw)
	if err != nil {
		return nil, err
	}
	if err := stack.Validate(); err != nil {
		return nil, fmt.Errorf("invalid stack: %w", err)
	}
	return stack, nil
}

// eval runs a Starlark config and returns the normalized stack as Go data
// (map/slice/scalar) ready to marshal into resource.Stack JSON. The config must
// call define_stack(...) exactly once.
func eval(ctx context.Context, path string, src []byte) (any, error) {
	h := &holder{}
	thread := &starlark.Thread{Name: "pando-config"}
	thread.SetLocal("ctx", ctx)
	opts := &syntax.FileOptions{Set: true, While: true, TopLevelControl: true}
	if _, err := starlark.ExecFileOptions(opts, thread, path, src, builtins(h)); err != nil {
		if ee, ok := err.(*starlark.EvalError); ok {
			return nil, fmt.Errorf("evaluate config: %s", ee.Backtrace())
		}
		return nil, fmt.Errorf("evaluate config: %w", err)
	}
	if !h.set {
		return nil, fmt.Errorf("config must call define_stack(...)")
	}
	return toGo(h.stack)
}

func decode(raw any) (*resource.Stack, error) {
	b, err := json.Marshal(raw)
	if err != nil {
		return nil, err
	}
	var stack resource.Stack
	dec := json.NewDecoder(bytes.NewReader(b))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&stack); err != nil {
		return nil, fmt.Errorf("decode stack: %w (got: %s)", err, truncate(b, 200))
	}
	return &stack, nil
}

func truncate(b []byte, n int) string {
	if len(b) > n {
		return string(b[:n]) + "..."
	}
	return string(b)
}
