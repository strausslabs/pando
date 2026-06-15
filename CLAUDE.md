# Pando — working conventions

Pando manages many git worktrees as one living dev environment: a Go daemon
(engine → scheduler → executors) plus a React/TS "grove" dashboard.

## Code style

These apply to **both** the Go backend and the TS frontend.

- **Match, not if/else ladders.** Replace `if/else if/else` chains with `switch`
  (Go) or a `switch`/lookup map (TS). A bare `switch { case cond: }` on a value,
  or `switch { case <bool>: }`, reads better than stacked `else if`.
- **Early returns.** Guard and return at the top; never wrap the body of a
  function in an `else`. Keep the happy path at the lowest indentation.
- **Inline single-use trivial helpers.** A one-line helper called from one place
  earns its name only if the name adds meaning. Otherwise inline it.
- **File order: constants, vars, structs, public funcs, private funcs.** Within a
  Go file, declare in that order. Promote a struct to a named type only if it is
  used more than once; a shape used in exactly one place is inlined (an anonymous
  struct or the literal at its use site), not named. Don't leave section-divider
  comments behind — the ordering itself is the structure.
- **Return shapes match their callers.** Return the type the caller actually
  uses (`(T, bool)` / `(T, error)` consistently); don't return a struct the
  caller immediately destructures into the same fields every time.
- **Code is self-explanatory; comments are the exception.** Default to none.
  Names carry the meaning — rename rather than annotate. The only comment that
  earns its place is a terse one-liner stating a non-obvious **invariant** or
  **footgun** whose violation silently breaks things and which the code itself
  cannot express. No narration, no section dividers, no restating the next line,
  no obvious doc comments. When in doubt, delete. (Functional directives like
  `//go:embed`, `//go:build`, `//nolint`, and `// eslint-disable-*` are not
  comments in this sense — keep them.)

## Architecture

- **Fix from the root, not patch-on-patch.** When you find a wrong pattern or
  the wrong abstraction, fix the cause even if the task grows. A clean codebase
  pays off far longer than a quick patch layered on a broken design. Don't stack
  a workaround on top of something that should be corrected — correct it.
- **Own mistakes; fix the code, not the check.** When a linter, type checker, or
  test fails, the default is that it found a real problem — fix the code. Do not
  disable the rule, loosen the config, add a suppression, or wave it off as a
  "false positive" to make the signal go away. If a rule is genuinely wrong for
  this repo, say so explicitly and get agreement before relaxing it; the burden
  is on the change to the check, not on keeping the code honest. If a previous
  step was wrong, name it and correct it rather than building on it.
- **Pure logic is extracted and testable.** Keep formatting, filtering, parsing,
  and ordering in plain functions, separate from React components and from the
  scheduler's control flow, so they can be unit-tested without a DOM or a daemon.

## Invariants (don't break these)

- Log buffer keys join `worktree` + `resource` with a **NUL byte** (`\0`), in
  both `useLogStore.ts` and `LogView.tsx`'s React key. Never change the
  separator — it cannot appear in a name, which is the whole point.
- Log lines carry a **process-global sequence number**; React keys and dedup use
  `worktree\0resource\0seq`. A bare `seq` collides across resources in the
  merged view.
- **Shared resources may depend only on other shared resources.** Enforced in
  `internal/engine/shared.go` (`mergeShared`) and resource validation.
- Worktree ordering is by **branch name** then slug — never by HEAD (it changes
  on every commit).

## Build & test

- Backend: `go build ./...`, `go vet ./...`, `golangci-lint run`, and
  `go test ./...` from the repo root. Exclude the vendored `ui/node_modules`
  Go package: `go test $(go list ./... | grep -v /ui/node_modules/)`.
- Frontend (in `ui/`): `bun run typecheck`, `bun run lint`, `bun run test:cov`,
  and `bun run build` (builds into `internal/web/dist` for `go:embed`).
- **Coverage is gated, not aspirational.** UI: `bun test --coverage` enforces a
  per-metric threshold via `bunfig.toml` (`App.tsx`/`Tree.tsx`/`main.tsx` are
  excluded — App is covered by the e2e suite). Go: Codecov's project status
  check gates the PR; `cmd/pando` (the `os.Exit` wrapper) is the only thing in
  the ignore list. Don't pad the ignore list to hit a number — cover the code or
  state plainly why a path is genuinely untestable (process spawn, signal
  handlers, real fs-watch loops).
- **UI e2e replays a real backend.** `ui/e2e/record/record.go` boots a real
  daemon over a host-process stack and snapshots each endpoint + `/events`
  frames into `ui/e2e/fixtures/`; `bun run e2e` (Playwright) replays them
  against the built UI. Regenerate fixtures with `bun run e2e:record` whenever
  the wire shape changes — never hand-edit them.
- **Pre-commit mirrors CI locally.** Install once with `pre-commit install &&
  pre-commit install --hook-type pre-push`. Commit runs the fast checks (format,
  vet, lint, `go test -short`, UI typecheck/lint/coverage); push runs the full
  `go test -race` suite. The Playwright e2e job runs in CI only.
- Work on a branch and open a PR; CI gates the merge. Don't commit straight to
  `main`. Do **not** mention Claude / Claude Code or add `Co-Authored-By`
  trailers.

## Development lifecycle

- **Every push to main** runs `ci.yml`: gofmt, vet, `go test -race`, build, and
  the UI (typecheck + bun test). This is the gate; it publishes nothing.
- **Daily release** (`release.yml`, cron 09:00 UTC, or manual dispatch): if there
  are commits since the last `v*` tag it bumps the patch (vX.Y.Z → vX.Y.(Z+1)),
  cross-compiles `CGO_ENABLED=0` static binaries for darwin/arm64, linux/amd64
  and linux/arm64 (UI built first so go:embed has assets), and cuts a GitHub
  release with tarballs + checksums. If nothing changed since the last tag, it
  skips — no empty release.
- Version is injected via `-ldflags -X main.version=<tag>`; `pando --version`
  reflects it. Local/dev builds report `dev`.
- **Homebrew tap.** Each release regenerates `Formula/pando.rb` and pushes it to
  the `guyStrauss/homebrew-pando` repo, so `brew install guyStrauss/pando/pando`
  always tracks the latest release. One-time setup: create that repo, and add a
  `TAP_TOKEN` secret (a PAT with contents:write on it) to this repo. Without the
  secret the tap step no-ops; the release still publishes.
