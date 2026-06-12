package config

import (
	"bytes"
	"context"
	_ "embed"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"

	"github.com/guyStrauss/pando/internal/resource"
)

//go:embed runtime.ts
var runtimeTS string

//go:embed types.ts
var typesTS string

type Loader struct {
	bunPath string
}

func NewLoader() (*Loader, error) {
	path, err := exec.LookPath("bun")
	if err != nil {
		return nil, fmt.Errorf("bun not found on PATH: %w", err)
	}
	return &Loader{bunPath: path}, nil
}

func (l *Loader) LoadFile(ctx context.Context, path string) (*resource.Stack, error) {
	abs, err := filepath.Abs(path)
	if err != nil {
		return nil, err
	}
	if _, err := os.Stat(abs); err != nil {
		return nil, fmt.Errorf("config not found: %w", err)
	}
	return l.run(ctx, filepath.Dir(abs), abs)
}

func (l *Loader) run(ctx context.Context, configDir, configPath string) (*resource.Stack, error) {
	tmp, err := os.MkdirTemp("", "pando-config-*")
	if err != nil {
		return nil, err
	}
	defer os.RemoveAll(tmp)

	if err := os.WriteFile(filepath.Join(tmp, "types.ts"), []byte(typesTS), 0o600); err != nil {
		return nil, err
	}
	if err := os.WriteFile(filepath.Join(tmp, "runtime.ts"), []byte(runtimeTS), 0o600); err != nil {
		return nil, err
	}
	entry := buildEntrypoint(configPath)
	entryPath := filepath.Join(tmp, "entry.ts")
	if err := os.WriteFile(entryPath, []byte(entry), 0o600); err != nil {
		return nil, err
	}

	var stdout, stderr bytes.Buffer
	cmd := exec.CommandContext(ctx, l.bunPath, "run", entryPath)
	cmd.Dir = configDir
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		return nil, fmt.Errorf("evaluate config: %w\n%s", err, stderr.String())
	}

	stack, err := decode(stdout.Bytes())
	if err != nil {
		return nil, err
	}
	if err := stack.Validate(); err != nil {
		return nil, fmt.Errorf("invalid stack: %w", err)
	}
	return stack, nil
}

func buildEntrypoint(configPath string) string {
	cfg, _ := json.Marshal(configPath)
	return fmt.Sprintf(`import { normalize } from "./runtime";
import * as mod from %s;
const raw = (mod as any).default ?? (globalThis as any).__pando_stack;
process.stdout.write(JSON.stringify(normalize(raw)));
`, cfg)
}

func decode(specJSON []byte) (*resource.Stack, error) {
	var stack resource.Stack
	dec := json.NewDecoder(bytes.NewReader(specJSON))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&stack); err != nil {
		return nil, fmt.Errorf("decode spec: %w (got: %s)", err, truncate(specJSON, 200))
	}
	return &stack, nil
}

func truncate(b []byte, n int) string {
	if len(b) > n {
		return string(b[:n]) + "..."
	}
	return string(b)
}
