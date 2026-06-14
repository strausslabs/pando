---
name: pando-star
description: >-
  Author, migrate, and debug pando.star configs for Pando, the multi-worktree
  dev environment manager. Use when a user wants to create or edit a pando.star
  file, move an existing dev setup (docker-compose, Procfile, Makefile, foreman,
  a pile of `npm run dev` terminals) onto Pando, set up dependency graphs /
  readiness probes / live-update / shared resources, or troubleshoot a stack
  that won't come up.
---

# Working with `pando.star`

> **Install this skill** (one line — pulls it into Claude Code's skill dir):
> ```sh
> mkdir -p ~/.claude/skills/pando-star && curl -fsSL https://raw.githubusercontent.com/strausslabs/pando/main/docs/pando-star-skill/SKILL.md -o ~/.claude/skills/pando-star/SKILL.md
> ```
> Then Claude auto-discovers it whenever you ask it to write or fix a
> `pando.star`. Verify with `/skills`.

`pando.star` describes one repo's dev environment for Pando. It is
[Starlark](https://github.com/bazelbuild/starlark): Python-like, deterministic,
**no `import`** — every helper is predeclared. The file must call
`define_stack(...)` exactly once.

```python
define_stack(
    name = "myapp",
    services = {
        "api": service(local = cmd("go run ./cmd/api")),
    },
)
```

> **The #1 mistake:** writing `import`. There are no imports. Every helper
> (`define_stack`, `service`, `cmd`, `task`, `compose`, `build`, `healthcheck`,
> `http_get`, `tcp`, `log_match`, `exit0`, `sync`, `run`, `restart`, `duration`,
> `bytes`) is already in scope. Just call it.

---

## Migrating an existing setup

Pick the source the project already has and translate it. Do this in three
passes: **(1)** one resource per process/container, **(2)** wire `deps` so things
start in order, **(3)** add a `ready` probe to each so dependents wait for *real*
readiness, not just "process started".

### From `docker-compose.yml`

Each compose service becomes a `service(compose = compose(...))`. Map fields
directly; swap fixed host ports for `$PORT_<name>` so worktrees don't collide.

```yaml
# docker-compose.yml
services:
  db:
    image: postgres:16
    ports: ["5432:5432"]
    environment: { POSTGRES_PASSWORD: dev }
  api:
    build: .
    ports: ["8080:8080"]
    depends_on: [db]
```

```python
define_stack(
    name = "myapp",
    services = {
        "db": service(
            compose = compose(image = "postgres:16", ports = ["$PORT_db:5432"],
                              env = {"POSTGRES_PASSWORD": "dev"}),
            ready = tcp("localhost:$PORT_db", timeout = "30s"),
        ),
        "api": service(
            build = build(context = "."),
            compose = compose(ports = ["$PORT_api:8080"]),
            deps = ["db"],
            ready = http_get("http://localhost:$PORT_api/health", timeout = "30s"),
        ),
    },
)
```

Field map: `image`→`image`, `ports`→`ports` (use `$PORT_*`), `environment`→`env`,
`env_file`→`envFile`, `volumes`→`volumes`, `command`→`command`,
`depends_on`→`deps`, `build:`→`build(context=...)`, `healthcheck`→`healthcheck(...)`,
`mem_limit`→`memory`, `cpus`→`cpus`, `restart`→`restart`.

### From a Procfile / foreman / `npm run dev` in many terminals

Each line (or each terminal you babysit) is a `local` process. Add `deps`/`ready`
where one waits on another.

```
# Procfile
web: npm run dev
api: go run ./cmd/api
css: npx tailwindcss -w
```

```python
define_stack(
    name = "myapp",
    services = {
        "api": service(
            local = cmd("go run ./cmd/api", env = {"PORT": "$PORT_api"}),
            ready = http_get("http://localhost:$PORT_api/health", timeout = "20s"),
            watch = ["./internal", "./cmd"],
        ),
        "web": service(
            local = cmd("npm run dev", cwd = "./web", env = {"API": "localhost:$PORT_api"}),
            deps = ["api"],
            ready = http_get("http://localhost:$PORT_web", timeout = "20s"),
        ),
        "css": service(local = cmd("npx tailwindcss -w", cwd = "./web")),
    },
)
```

### From a Makefile (`make dev`, `make migrate`)

One-shot targets (build, migrate, seed, codegen) become `task`; long-running
targets become `local`. Order them with `deps`.

```python
"migrate": service(task = task(cmd = "make migrate"), deps = ["db"], runWhen = "once"),
"api":     service(local = cmd("make run"), deps = ["migrate"]),
```

### Migration checklist

- [ ] One `service` per process/container.
- [ ] Every hardcoded host port replaced with `$PORT_<name>`.
- [ ] `depends_on` / "I start this terminal after that one" → `deps`.
- [ ] One-shot steps (migrate/seed/codegen) → `task` with `runWhen = "once"`.
- [ ] Each long-running service has a `ready` probe so dependents truly wait.
- [ ] Shared infra (a DB used by every branch) → `shared = True`.
- [ ] No `import`; `define_stack` called once.

---

## Authoring decision guide

1. **What runs it?** local binary → `local = cmd("...")`; container →
   `compose = compose(image="...")` (add `build = build(...)` to build); one-shot
   → `task = task(cmd="...")`.
2. **What comes first?** → `deps = [...]`.
3. **How do we know it's up?** → `ready =` (see Probes). Don't skip this — it's
   what makes the graph correct instead of a `sleep`.
4. **Ports:** never hardcode a host port. Use `$PORT_<resource>`; reference it in
   `ports`, `env`, and probe targets.
5. **Shared infra?** `shared = True` — brought up once for the whole repo. A
   shared resource may depend **only** on other shared resources.
6. **Fast container iteration?** `liveUpdate = [sync(...), run(...), restart()]`.

## Helper signatures

```python
cmd(cmd, cwd?, env?, watch?)          # host process; watch=[paths] restarts it
task(cmd, cwd?, env?)                 # one-shot; defaults runWhen="once"
compose(image?, ports?, env?, envFile?, dependsOn?, volumes?, command?,
        memory?, cpus?, pidsLimit?, restart?, healthcheck?)
build(context, dockerfile?, args?, target?, secrets?)
healthcheck(test, interval?, timeout?, retries?)

http_get(target, timeout?, interval?)
tcp(target, timeout?, interval?)
log_match(pattern, timeout?, interval?)   # pattern is a regex
exit0(timeout?, interval?)

sync(local, container)                # liveUpdate: copy files in
run(cmd, trigger?)                     # liveUpdate: run a command (trigger scopes it)
restart()                              # liveUpdate: restart the resource

duration("30s")  bytes("256m")        # explicit coercion; strings also work inline
```

`service(local=|task=|compose=, build?, deps?, ready?, runWhen?, onChange?,
every?, shared?, preview?, liveUpdate?, hooks?)`

- `runWhen`: `"once"` | `"always"` | `"onChange"` | `"manual"`. `local`/`compose`
  default `always`; `task` defaults `once`. `onChange=[paths]` is required when
  `runWhen="onChange"`. `every="30s"` runs periodically.
- Durations: `"500ms"`, `"30s"`, `"5m"`, `"1h"`. Sizes: `"256m"`, `"1g"`.

---

## Troubleshooting

Read the resource's own logs first — that's where the real error is:

```sh
pando status                          # phases + ports for every worktree
pando logs <name> -w <worktree> --tail 50
pando logs <name> -w <worktree> --grep 'error|panic|refused'
```

| Symptom | Likely cause | Fix |
|---------|--------------|-----|
| `config must call define_stack(...)` | file never calls it, or an exception threw first | Ensure exactly one top-level `define_stack(...)`. |
| `evaluate config: ... undefined: foo` | typo, or you wrote `import` | Remove imports; check the helper name against the list above. |
| `unknown field "x"` on a service | field misspelled or not a `service()` field | Compare against the `service(...)` signature; move compose-only fields inside `compose(...)`. |
| Resource stuck **not ready** / dependents never start | `ready` probe never passes | `pando logs <name>`; verify the probe target/port; the process may bind a different port than `$PORT_<name>`. |
| `bind: address already in use` | a host port is hardcoded instead of `$PORT_*`, or a stale process holds it | Use `$PORT_<name>` everywhere; `pando down` to clear, then `pando up`. |
| Probe passes but app actually broken | probing the wrong thing (e.g. TCP open but app 500s) | Use `http_get` on a real `/health` route, not `tcp`. |
| Task reruns every time | `task` without `runWhen` defaults sensibly, but a `local` won't stop | One-shots → `task(...)` + `runWhen="once"`; force a rerun with `pando up --force`. |
| Shared resource rejected at validation | it depends on a non-shared resource | Shared may depend only on shared. Make the dep shared too, or drop it. |
| Edit to `pando.star` did nothing | daemon didn't reload, or the diff was a no-op | Pando hot-reloads on save; if not, `pando down && pando up`. |
| `not inside a git repository` | running outside a worktree | `cd` into the repo; Pando keys everything off the git dir. |

When a resource fails, the fastest loop is: `pando logs <name> --tail 50` → fix
the `cmd`/`env`/`ready` in `pando.star` → save (hot reload) → `pando status`.

Full field reference: `docs/config.md` in the Pando repo.
