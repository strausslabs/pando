---
name: pando-star
description: >-
  Author and edit pando.star configuration files for Pando, the multi-worktree
  dev environment manager. Use when a user asks to create, edit, or debug a
  pando.star file, define resources/services for Pando, set up dependency graphs,
  readiness probes, live-update rules, or shared resources.
---

# Authoring `pando.star`

`pando.star` describes one repo's dev environment for Pando. It is
[Starlark](https://github.com/bazelbuild/starlark): Python-like, deterministic,
**no `import`** — every helper is predeclared. The file must call
`define_stack(...)` exactly once.

## The one rule that trips people up

There are **no imports and no `from ... import`**. If you write `import` you will
break the file. All helpers (`define_stack`, `service`, `cmd`, `task`,
`compose`, `build`, `healthcheck`, `http_get`, `tcp`, `log_match`, `exit0`,
`sync`, `run`, `restart`, `duration`, `bytes`) are already in scope. Just call
them.

## Skeleton

```python
define_stack(
    name = "<stack-name>",
    services = {
        "<resource-name>": service(<spec>, deps = [...], ready = <probe>, ...),
    },
)
```

- `services` keys are resource names — lowercase DNS-1123 (`a-z`, `0-9`, `-`).
- Each value is `service(...)`.
- Kind is **inferred**: `local=` → host process, `task=` → one-shot, else
  `compose=` → Docker service. Set exactly one.

## Decision guide

1. **What runs it?**
   - A local binary/dev server → `local = cmd("...")`.
   - A container → `compose = compose(image = "...")` (add `build = build(...)`
     to build instead of pull).
   - A one-shot step (migrations, codegen) → `task = task(cmd = "...")`.
2. **What must come first?** Put their names in `deps = [...]`.
3. **How do we know it's up?** Add `ready =`:
   - HTTP service → `http_get("http://localhost:$PORT_<name>/health", timeout = "30s")`
   - Plain port → `tcp("localhost:$PORT_<name>", timeout = "30s")`
   - Log sentinel → `log_match("listening on", timeout = "30s")`
   - One-shot success → `exit0()`
4. **Ports.** Never hardcode a host port. Use `$PORT_<resource>` — Pando
   allocates a unique one per worktree, so branches don't collide. Reference it
   in `ports`, `env`, and probe targets.
5. **Shared infra?** Mark `shared = True` to bring it up once for the whole repo.
   A shared resource may depend **only on other shared resources**.
6. **Fast iteration in containers?** Add `liveUpdate = [sync(...), run(...), restart()]`.

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

## Field cheatsheet

- `runWhen`: `"once"` | `"always"` | `"onChange"` | `"manual"`. `local`/`compose`
  default to `always`; `task` defaults to `once`.
- `onChange = [paths]` is **required** when `runWhen = "onChange"`.
- `every = "30s"` runs periodically (implies `always`).
- Durations: `"500ms"`, `"30s"`, `"5m"`, `"1h"`. Sizes: `"256m"`, `"1g"`.
- `hooks = {"postStart": "...", "preStop": "..."}`.

## Worked example

```python
define_stack(
    name = "shop",
    services = {
        "pg": service(
            compose = compose(image = "postgres:16", ports = ["$PORT_pg:5432"],
                              env = {"POSTGRES_PASSWORD": "dev"}),
            shared = True,
            ready = tcp("localhost:$PORT_pg", timeout = "30s"),
        ),
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

## Validation checklist before you finish

- [ ] No `import` statements.
- [ ] `define_stack` called once, with `name` and `services`.
- [ ] Every `service` sets exactly one of `local` / `task` / `compose`.
- [ ] Every name in some `deps` exists as a key in `services`.
- [ ] No hardcoded host ports — use `$PORT_<name>`.
- [ ] Shared resources depend only on shared resources.
- [ ] `onChange` present whenever `runWhen = "onChange"`.

Full reference: see `docs/config.md` in the Pando repo.
