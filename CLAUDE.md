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

- Backend: `go build ./...` and `go test ./...` from the repo root.
- Frontend (in `ui/`): `bun test`, `bun run typecheck`, `bun run build`
  (builds into `internal/web/dist` for `go:embed`).
- Commits go directly on `main`. Do **not** mention Claude / Claude Code or add
  `Co-Authored-By` trailers.
