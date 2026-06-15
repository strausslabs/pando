# `pando.star` configuration reference

Pando reads one file per worktree: `pando.star`. It is
[Starlark](https://github.com/bazelbuild/starlark) — Python-like, deterministic,
no `import`. Every helper below is predeclared in the global namespace, so you
just call it. The file must call `define_stack(...)` **exactly once**.

```python
define_stack(
    name = "myapp",
    services = {
        "api": service(local = cmd("go run ./cmd/api")),
    },
)
```

Because it's a real language you can use loops, conditionals, and local
variables to build the `services` dict — but the result must be static data by
the time `define_stack` returns.

---

## `define_stack`

The single entry point.

| field | type | required | meaning |
|-------|------|----------|---------|
| `name` | string | yes | Stack name. |
| `services` | dict | yes | Map of resource name → `service(...)`. The key is the resource name; it must be a valid DNS-1123 hostname (lowercase, digits, `-`). |

Each value in `services` is a `service(...)`. The resource's **kind** is inferred
from the spec it carries: `local=` → a host process, `task=` → a one-shot
command, otherwise `compose=` → a Docker Compose service.

---

## `service`

Wraps one resource. Exactly one of `local`, `task`, or `compose` should be set.

| field | type | meaning |
|-------|------|---------|
| `local` | `cmd(...)` | Run as a host process. |
| `task` | `task(...)` | Run a one-shot command (defaults to `runWhen = "once"`). |
| `compose` | `compose(...)` | Run as a Docker Compose service. |
| `build` | `build(...)` | Build an image for this service before running it. |
| `deps` | list[string] | Names this resource waits on before starting. |
| `ready` | probe | Readiness probe — Pando holds dependents until it passes. See [Probes](#probes). |
| `runWhen` | string | `"once"`, `"always"`, `"onChange"`, or `"manual"`. |
| `onChange` | list[string] | Glob/paths that trigger a rerun when `runWhen = "onChange"`. Required in that mode. |
| `every` | duration | Run periodically (e.g. `"30s"`). Implies `runWhen = "always"`. |
| `shared` | bool | Bring up **once for the whole repo**, reused across worktrees. Shared resources may depend only on other shared resources. |
| `liveUpdate` | list | In-place update steps; see [Live update](#live-update). |
| `hooks` | dict | `{"postStart": "...", "preStop": "..."}` shell hooks. |

---

## Resource specs

### `cmd` (host process) / `task` (one-shot)

Same signature; `cmd` is long-running, `task` runs to completion.

```python
cmd(
    cmd = "go run ./cmd/api",   # required: the command line
    cwd = "./api",              # optional: working dir
    env = {"LOG_LEVEL": "debug"},
    watch = ["./api"],          # optional: restart on changes here (cmd only)
)
```

| field | type | meaning |
|-------|------|---------|
| `cmd` | string | Command line (required). |
| `cwd` | string | Working directory, relative to the worktree. |
| `env` | dict[string,string] | Extra environment. |
| `watch` | list[string] | Paths that restart the process on change. |

### `compose` (Docker service)

```python
compose(
    image = "postgres:16",
    ports = ["$PORT_db:5432"],
    env = {"POSTGRES_PASSWORD": "dev"},
    volumes = ["pgdata:/var/lib/postgresql/data"],
    healthcheck = healthcheck(test = ["CMD", "pg_isready"], interval = "5s", retries = 5),
)
```

| field | type | meaning |
|-------|------|---------|
| `image` | string | Image ref. Omit when building via `build(...)`. |
| `ports` | list[string] | `"host:container"`. Use `$PORT_<name>` for an auto-allocated host port. |
| `env` | dict[string,string] | Environment. |
| `envFile` | list[string] | Env files to load. |
| `dependsOn` | list[string] | Compose-level dependencies (merged with `deps`). |
| `volumes` | list[string] | Volume mounts. |
| `command` | list[string] | Override the image command. |
| `memory` | int / `bytes(...)` | Memory limit. |
| `cpus` | float | CPU limit. |
| `pidsLimit` | int | PID limit. |
| `restart` | string | `no` / `on-failure` / `always` / `unless-stopped`. |
| `healthcheck` | `healthcheck(...)` | Container healthcheck. |

### `build` (image build)

```python
build(
    context = "./api",
    dockerfile = "Dockerfile",
    args = {"VERSION": "dev"},
    target = "runtime",
)
```

`context` is required. `secrets` is a list of `{"id": ..., "src": ...}` dicts.

### `healthcheck`

```python
healthcheck(test = ["CMD", "curl", "-f", "http://localhost/health"],
            interval = "5s", timeout = "3s", retries = 5)
```

`test` is required (a list — the Compose healthcheck test). `interval`/`timeout`
are durations; `retries` an int.

---

## Probes

Readiness probes for `ready =`. Pando keeps dependents blocked until the probe
passes.

| helper | signature | passes when |
|--------|-----------|-------------|
| `http_get` | `http_get(target, timeout?, interval?)` | `target` URL returns 2xx. |
| `tcp` | `tcp(target, timeout?, interval?)` | TCP connect to `target` (`host:port`) succeeds. |
| `log_match` | `log_match(pattern, timeout?, interval?)` | a log line matches the regex `pattern`. |
| `exit0` | `exit0(timeout?, interval?)` | the process exits 0 (for tasks). |

```python
ready = http_get("http://localhost:$PORT_api/health", timeout = "30s", interval = "1s")
```

`timeout` and `interval` accept a duration string or a `duration(...)` value.

---

## Live update

In-place updates to a running resource without a full restart — ordered steps:

| step | signature | does |
|------|-----------|------|
| `sync` | `sync(local, container)` | Copy `local` path into the container at `container`. |
| `run` | `run(cmd, trigger?)` | Run `cmd` inside the resource. `trigger` (string or list) scopes it to changes under those paths. |
| `restart` | `restart()` | Restart the resource. |

```python
liveUpdate = [
    sync("./api", "/app"),
    run("go build -o /app/server ./cmd/api", trigger = ["./api"]),
    restart(),
]
```

---

## Durations & sizes

Anywhere a duration is accepted you can pass a string (`"500ms"`, `"30s"`,
`"5m"`, `"1h"`) or `duration("30s")`. Sizes accept `"256m"`, `"1g"`, `"512kb"`,
or `bytes("256m")`. Bare ints are treated as nanoseconds / bytes respectively.

## Port variables

`$PORT_<resource>` expands to a host port Pando allocates per worktree, so the
same config on three branches never collides. Reference it in `ports`, env
values, and probe targets.

## `runWhen` semantics

| value | behavior |
|-------|----------|
| `once` | Run a single time (default for `task`). |
| `always` | Keep running / restart on exit (default for `local`/`compose`). |
| `onChange` | Rerun when files under `onChange` change. |
| `manual` | Only via `pando up <name>` / the dashboard. |

## A fuller example

```python
SHARED_PG = service(
    compose = compose(image = "postgres:16", ports = ["$PORT_pg:5432"],
                      env = {"POSTGRES_PASSWORD": "dev"}),
    shared = True,
    ready = tcp("localhost:$PORT_pg", timeout = "30s"),
)

define_stack(
    name = "shop",
    services = {
        "pg": SHARED_PG,
        "migrate": service(task = task(cmd = "./bin/migrate"), deps = ["pg"]),
        "api": service(
            local = cmd("go run ./cmd/api", env = {"DB": "localhost:$PORT_pg"}),
            deps = ["migrate"],
            ready = http_get("http://localhost:$PORT_api/healthz", timeout = "20s"),
            watch = ["./internal", "./cmd"],
        ),
        "web": service(
            local = cmd("bun run dev", cwd = "./web"),
            deps = ["api"],
            ready = http_get("http://localhost:$PORT_web", timeout = "20s"),
        ),
    },
)
```
