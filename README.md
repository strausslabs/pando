<img src="assets/logo.svg" alt="Pando" width="96" align="right" />

# Pando

[![CI](https://github.com/strausslabs/pando/actions/workflows/ci.yml/badge.svg)](https://github.com/strausslabs/pando/actions/workflows/ci.yml)
[![codecov](https://codecov.io/gh/strausslabs/pando/branch/main/graph/badge.svg)](https://codecov.io/gh/strausslabs/pando)
[![Release](https://img.shields.io/github/v/release/strausslabs/pando?sort=semver)](https://github.com/strausslabs/pando/releases)
[![Go Reference](https://pkg.go.dev/badge/github.com/guyStrauss/pando.svg)](https://pkg.go.dev/github.com/guyStrauss/pando)

**Run every git worktree of a repo as one living dev environment.** One daemon
turns each worktree's `pando.star` into a set of resources — host processes,
Docker Compose services and one-shot tasks — wires them into a dependency graph,
brings them up in order, streams their logs, hot-reloads on config change, and
shows the whole grove in a web dashboard. Branch away.

<p align="center">
  <img src="assets/demo.gif" alt="Pando demo" width="760" />
</p>

## Why

Working on three branches at once usually means three clones, three sets of
ports that collide, and three `docker compose up` terminals you forgot to stop.
Pando treats the worktrees of a single repo as first-class: each gets isolated,
auto-allocated ports (`$PORT_<name>`), its own resource graph, and shared
resources (a database, say) that come up once and are reused across branches.

- **Dependency graph, not a script.** Declare `deps`; Pando topologically orders
  startup, waits on readiness probes, and tears down in reverse.
- **Live config.** Edit `pando.star`, save — the daemon diffs the stack and
  restarts only what changed.
- **Live update.** `sync` files into a running container and `run` a rebuild
  without a full restart.
- **Shared resources.** Mark a resource `shared` and it's brought up once for the
  whole repo, not per worktree.
- **Agent-native.** A built-in MCP server lets an AI agent inspect status, search
  logs, and drive resources. See [Agents](#agents-mcp).

## Install

```sh
brew tap strausslabs/pando https://github.com/strausslabs/pando
brew install strausslabs/pando/pando
```

Or grab a static binary from [releases](https://github.com/strausslabs/pando/releases)
(darwin/arm64, linux/amd64, linux/arm64).

## Quick start

```sh
cd your-repo
$EDITOR pando.star   # describe your resources
pando up             # starts the daemon + dashboard, brings this worktree up
```

`pando up` auto-starts a per-repo daemon if one isn't already running, then
brings the worktree up. The dashboard binds a **port derived from the repo**
(7420 + an offset of the repo's identity), so every worktree of a repo shares
one dashboard and a second repo gets its own — run the same repo twice and the
per-repo socket makes the duplicate a no-op. Use `pando start` to run the daemon
in the foreground instead.

A minimal `pando.star` (Starlark — no imports, every helper is built in):

```python
define_stack(
    name = "myapp",
    services = {
        "db": service(compose = compose(image = "postgres:16", ports = ["$PORT_db:5432"])),
        "migrate": service(task = task(cmd = "./migrate"), deps = ["db"], runWhen = "once"),
        "api": service(
            local = cmd("go run ./cmd/api"),
            deps = ["migrate"],
            ready = http_get("http://localhost:$PORT_api/health", timeout = "30s"),
            liveUpdate = [sync("./api", "/app"), run("go build"), restart()],
        ),
    },
)
```

Full syntax: **[Config reference](docs/config.md)**. Editing configs with an AI
agent? Install the **[pando.star skill](docs/pando-star-skill/SKILL.md)**.

## Agents (MCP)

Pando speaks the Model Context Protocol so an agent can drive it:

```sh
claude mcp add pando -- pando mcp
```

Tools: `pando_running`, `pando_status`, `pando_logs`, `pando_logs_search`
(regex + tail), `pando_exec`, `pando_up`, `pando_down`, `pando_restart`. Each
command and the MCP server auto-discover the daemon for the current repo, so
multiple repos can run Pando at once.

## CLI

`pando up` (auto-starts the daemon) · `pando start` (foreground daemon) ·
`pando status` · `pando logs <resource>` · `pando exec <resource> -- <cmd>` ·
`pando restart <resource>` · `pando down` · `pando worktrees`. Add `--json` for
machine-readable output.

## Development

Backend: `go build ./...` and `go test ./...` from the repo root. Frontend (in
`ui/`): `bun test`, `bun run typecheck`, `bun run build` (builds into
`internal/web/dist` for `go:embed`). CI runs all of this on every push to main;
a daily release auto-bumps the patch version when there are new commits.
