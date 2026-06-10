import { memo, useMemo } from "react";
import type { Phase, WorktreeStatus } from "./types";
import { phaseColor, phaseLabel } from "./phase";

type Target =
  | { kind: "resource"; worktree: string; resource: string }
  | { kind: "worktree"; worktree: string };

const DONE = new Set<Phase>(["done", "skipped", "stopped"]);

// Human-readable byte size: base 1024, 0 decimals at >=10 of a unit, 1 below.
function formatBytes(n: number): string {
  const units = ["B", "KB", "MB", "GB", "TB"];
  let v = n;
  let i = 0;
  while (v >= 1024 && i < units.length - 1) {
    v /= 1024;
    i++;
  }
  const digits = v >= 10 || i === 0 ? 0 : 1;
  return `${v.toFixed(digits)} ${units[i]}`;
}

// CPU share of one core: 1 decimal under 10% so quiet processes don't read 0%.
function formatCpu(pct: number): string {
  return pct < 10 ? `${pct.toFixed(1)}%` : `${pct.toFixed(0)}%`;
}

// Compact interval label: "30s" / "5m" / "2h" / "1d".
function formatEvery(seconds: number): string {
  if (seconds < 60) return `${Math.round(seconds)}s`;
  if (seconds < 3600) return `${Math.round(seconds / 60)}m`;
  if (seconds < 86400) return `${Math.round(seconds / 3600)}h`;
  return `${Math.round(seconds / 86400)}d`;
}

interface Props {
  searchRef?: React.Ref<HTMLInputElement>;
  stacks: WorktreeStatus[];
  target: Target | null;
  filter: string;
  hideDone: boolean;
  onFilter: (v: string) => void;
  onToggleHideDone: () => void;
  onSelect: (t: Target) => void;
  onUp: (worktree: string) => void;
  onDown: (worktree: string) => void;
  onRestart: (worktree: string, resource: string) => void;
}

export function Sidebar(props: Props) {
  const { stacks, filter, hideDone } = props;

  // Filter worktrees -> matching resources by free text over both names. A
  // worktree shows if its own name matches (all resources) or any resource
  // matches (just those).
  const view = useMemo(() => {
    const q = filter.trim().toLowerCase();
    return stacks
      .map((ws) => {
        const wtMatch = ws.worktree.toLowerCase().includes(q);
        let resources = ws.resources;
        if (q && !wtMatch) resources = resources.filter((r) => r.name.toLowerCase().includes(q));
        if (hideDone) {
          const kept = resources.filter((r) => !DONE.has(r.phase));
          // Keep done resources only if the search explicitly matched them.
          resources = q && !wtMatch ? resources : kept;
        }
        return { ws, resources };
      })
      .filter((g) => g.resources.length > 0 || (!filter && g.ws.resources.length === 0));
  }, [stacks, filter, hideDone]);

  const hiddenCount = useMemo(() => {
    if (!hideDone) return 0;
    let n = 0;
    for (const ws of stacks) for (const r of ws.resources) if (DONE.has(r.phase)) n++;
    return n;
  }, [stacks, hideDone]);

  return (
    <nav className="sidebar">
      <div className="search">
        <input
          ref={props.searchRef}
          className="search-input"
          placeholder="filter worktrees & resources…"
          value={filter}
          onChange={(e) => props.onFilter(e.target.value)}
          spellCheck={false}
        />
        {filter ? (
          <button className="search-clear" onClick={() => props.onFilter("")}>
            ×
          </button>
        ) : (
          <kbd className="search-hint">⌘K</kbd>
        )}
      </div>

      <button className={`toggle ${hideDone ? "on" : ""}`} onClick={props.onToggleHideDone}>
        <span className="toggle-box">{hideDone ? "✓" : ""}</span>
        hide finished
        {hiddenCount > 0 ? <span className="toggle-count">{hiddenCount}</span> : null}
      </button>

      <div className="tree">
        {view.length === 0 ? (
          <div className="tree-empty">no matches</div>
        ) : (
          view.map(({ ws, resources }) => (
            <StemGroup
              key={ws.worktree}
              ws={ws}
              resources={resources}
              target={props.target}
              onSelect={props.onSelect}
              onUp={props.onUp}
              onDown={props.onDown}
              onRestart={props.onRestart}
            />
          ))
        )}
      </div>
    </nav>
  );
}

const StemGroup = memo(function StemGroup({
  ws,
  resources,
  target,
  onSelect,
  onUp,
  onDown,
  onRestart,
}: {
  ws: WorktreeStatus;
  resources: WorktreeStatus["resources"];
  target: Target | null;
  onSelect: (t: Target) => void;
  onUp: (w: string) => void;
  onDown: (w: string) => void;
  onRestart: (w: string, r: string) => void;
}) {
  const allSelected = target?.kind === "worktree" && target.worktree === ws.worktree;
  return (
    <section className="stem">
      <header className="stem-head">
        <button
          className={`stem-branch ${allSelected ? "active" : ""}`}
          onClick={() => onSelect({ kind: "worktree", worktree: ws.worktree })}
          title="show all logs from this worktree"
        >
          {ws.branch || ws.worktree}
        </button>
        {ws.head ? (
          <span className="stem-slug" title={`HEAD ${ws.head}`}>
            {ws.head.slice(0, 7)}
          </span>
        ) : null}
        <div className="stem-actions">
          <button className="btn" onClick={() => onUp(ws.worktree)}>
            up
          </button>
          <button className="btn ghost" onClick={() => onDown(ws.worktree)}>
            down
          </button>
        </div>
      </header>
      <ul className="leaves">
        {resources.map((r) => {
          const selected =
            target?.kind === "resource" && target.worktree === ws.worktree && target.resource === r.name;
          return (
            <li
              key={r.name}
              className={`leaf-row ${selected ? "selected" : ""}`}
              data-state={r.phase}
              onClick={() => onSelect({ kind: "resource", worktree: ws.worktree, resource: r.name })}
            >
              <span className="glyph" style={{ ["--phase" as string]: phaseColor(r.phase) }} />
              <span className="leaf-name">{r.name}</span>
              <span className="leaf-vitals" title="live footprint">
                {r.cpuPercent ? <span className="leaf-cpu">{formatCpu(r.cpuPercent)}</span> : null}
                {r.memBytes ? <span className="leaf-mem">{formatBytes(r.memBytes)}</span> : null}
                {r.memLimitBytes ? (
                  <span className="leaf-memlimit">/ {formatBytes(r.memLimitBytes)}</span>
                ) : null}
              </span>
              <span className="leaf-phase">{phaseLabel(r.phase)}</span>
              {r.everySeconds ? (
                <span
                  className="leaf-every"
                  title={
                    r.nextRunUnix ? `next run ${new Date(r.nextRunUnix * 1000).toLocaleTimeString()}` : undefined
                  }
                >
                  ⟳ {formatEvery(r.everySeconds)}
                </span>
              ) : null}
              {r.port ? <span className="leaf-port">:{r.port}</span> : null}
              <button
                className="leaf-restart"
                title="restart"
                onClick={(e) => {
                  e.stopPropagation();
                  onRestart(ws.worktree, r.name);
                }}
              >
                ↻
              </button>
            </li>
          );
        })}
      </ul>
    </section>
  );
});
