import { memo, useMemo } from "react";
import type { Phase, WorktreeStatus } from "./types";
import { phaseColor, phaseLabel } from "./phase";
import { bufferKey } from "./logbuffer";
import { formatBytes, formatCpu, formatEvery } from "./format";

type Target =
  | { kind: "resource"; worktree: string; resource: string }
  | { kind: "worktree"; worktree: string };

const DONE = new Set<Phase>(["done", "skipped", "stopped"]);

interface Props {
  searchRef?: React.Ref<HTMLInputElement>;
  stacks: WorktreeStatus[];
  target: Target | null;
  filter: string;
  flashing?: Set<string>;
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

  const view = useMemo(() => {
    const q = filter.trim().toLowerCase();
    return stacks
      .map((ws) => {
        const wtMatch = ws.worktree.toLowerCase().includes(q);
        let resources = ws.resources;
        if (q && !wtMatch) resources = resources.filter((r) => r.name.toLowerCase().includes(q));
        if (hideDone) {
          const kept = resources.filter((r) => !DONE.has(r.phase));
          resources = q && !wtMatch ? resources : kept;
        }
        return { ws, resources };
      })
      .filter((g) => g.resources.length > 0 || g.ws.error || (!filter && g.ws.resources.length === 0));
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
              flashing={props.flashing}
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
  flashing,
  onSelect,
  onUp,
  onDown,
  onRestart,
}: {
  ws: WorktreeStatus;
  resources: WorktreeStatus["resources"];
  target: Target | null;
  flashing?: Set<string>;
  onSelect: (t: Target) => void;
  onUp: (w: string) => void;
  onDown: (w: string) => void;
  onRestart: (w: string, r: string) => void;
}) {
  const allSelected = target?.kind === "worktree" && target.worktree === ws.worktree;
  return (
    <section className={`stem ${ws.error ? "blighted" : ""}`}>
      <header className="stem-head">
        <button
          className={`stem-branch ${allSelected ? "active" : ""}`}
          onClick={() => onSelect({ kind: "worktree", worktree: ws.worktree })}
          title="show all logs from this worktree"
        >
          {ws.error ? <span className="stem-blight-mark" aria-hidden /> : null}
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
      {ws.error ? (
        <div className="stem-blight" role="alert" title={ws.error}>
          <span className="stem-blight-label">config blight</span>
          <span className="stem-blight-msg">{ws.error}</span>
        </div>
      ) : null}
      <ul className="leaves">
        {resources.map((r) => {
          const selected =
            target?.kind === "resource" && target.worktree === ws.worktree && target.resource === r.name;
          const isFlashing = flashing?.has(bufferKey(ws.worktree, r.name)) ?? false;
          return (
            <li
              key={r.name}
              className={`leaf-row ${selected ? "selected" : ""} ${isFlashing ? "flash" : ""}`}
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
