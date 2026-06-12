<img src="assets/logo.svg" alt="Pando" width="96" align="right" />

# Pando

Fast multi-worktree dev environments. One daemon manages every git worktree of a
repo as a living set of resources — host processes, Docker Compose services and
one-shot tasks — with a dependency graph, live logs, hot config reload and a web
dashboard. Branch away.

## Install

```sh
brew install guyStrauss/pando/pando
```

Or grab a static binary from [releases](https://github.com/guyStrauss/pando/releases)
(darwin/arm64, linux/amd64, linux/arm64).

## Quick start

```sh
cd your-repo
$EDITOR pando.star   # describe your resources
pando start          # daemon + dashboard at http://127.0.0.1:7420
```

A `pando.star` config (Starlark — no imports, helpers are built in):

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

`pando start` · `pando status` · `pando logs <resource>` · `pando exec <resource> -- <cmd>` ·
`pando restart <resource>` · `pando up` / `pando down` · `pando worktrees`. Add
`--json` for machine-readable output.
