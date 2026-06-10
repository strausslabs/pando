import { useCallback, useEffect, useMemo, useRef, useState } from "react";
import type { Phase, WireEvent, WorktreeStatus } from "./types";
import { api } from "./api";
import { useEvents } from "./useEvents";
import { useLogStore } from "./useLogStore";
import { useHotkeys } from "./useHotkeys";
import { LogView } from "./LogView";
import { Preview } from "./Preview";
import { Sidebar } from "./Sidebar";
import { Tree, Mark } from "./Tree";
import { Shortcuts } from "./Shortcuts";
import { phaseLabel } from "./phase";

// A log target is either a single resource or every resource in a worktree.
type Target =
  | { kind: "resource"; worktree: string; resource: string }
  | { kind: "worktree"; worktree: string };

const DONE = new Set<Phase>(["done", "skipped", "stopped"]);

export function App() {
  const [stacks, setStacks] = useState<WorktreeStatus[]>([]);
  const [target, setTarget] = useState<Target | null>(null);
  const [filter, setFilter] = useState(""); // grove search (worktree/resource names)
  const [logQuery, setLogQuery] = useState(""); // in-log free-text search
  const [hideDone, setHideDone] = useState(true);
  const [snap, setSnap] = useState(true); // follow the tail by default
  const [showPreview, setShowPreview] = useState(true); // prefer the live app over logs
  const [showHelp, setShowHelp] = useState(false);
  const searchRef = useRef<HTMLInputElement>(null);
  const logSearchRef = useRef<HTMLInputElement>(null);
  const { append, linesFor, linesForWorktree, version } = useLogStore();

  useHotkeys({
    onCommandK: () => searchRef.current?.focus(),
    onCommandL: () => logSearchRef.current?.focus(),
    onHelp: () => setShowHelp((v) => !v),
    onEscape: () => setShowHelp(false),
  });

  const applyPhase = useCallback((worktree: string, resource: string, phase: Phase) => {
    setStacks((prev) =>
      prev.map((ws) =>
        ws.worktree !== worktree
          ? ws
          : {
              ...ws,
              resources: ws.resources.map((r) =>
                r.name === resource ? { ...r, phase, ready: phaseReady(phase) } : r,
              ),
            },
      ),
    );
  }, []);

  const onEvent = useCallback(
    (ev: WireEvent) => {
      if (ev.kind === "log" && ev.line) append(ev.line);
      else if (ev.kind === "phase" && ev.phase !== undefined) applyPhase(ev.worktree, ev.resource, ev.phase);
    },
    [append, applyPhase],
  );
  const { connected } = useEvents(onEvent);

  const refresh = useCallback(async () => {
    try {
      const st = await api.status();
      setStacks(st);
      setTarget((cur) => cur ?? firstResource(st));
    } catch {
      /* daemon not up yet; poll retries */
    }
  }, []);

  useEffect(() => {
    refresh();
    const t = setInterval(refresh, 4000);
    return () => clearInterval(t);
  }, [refresh]);

  // stacksRef lets the backfill read the current resource list without listing
  // `stacks` as an effect dependency — otherwise every phase update would
  // re-run the effect and cancel in-flight fetches (which broke the merged
  // worktree view, where six fetches race against constant phase churn).
  const stacksRef = useRef<WorktreeStatus[]>([]);
  stacksRef.current = stacks;

  // Backfill historical logs on selection. Live WS events only carry lines
  // emitted after the socket connected, so a settled resource (a finished task)
  // would otherwise show nothing. append() dedups by seq, so backfill and live
  // lines merge without duplicates.
  useEffect(() => {
    if (!target) return;
    const fetchFor = async (worktree: string, resource: string) => {
      try {
        const logs = await api.logs(worktree, resource, 1000);
        logs.forEach(append);
      } catch {
        /* ignore; live stream still works */
      }
    };
    if (target.kind === "resource") {
      fetchFor(target.worktree, target.resource);
    } else {
      const ws = stacksRef.current.find((s) => s.worktree === target.worktree);
      ws?.resources.forEach((r) => fetchFor(target.worktree, r.name));
    }
  }, [target, append]);

  // When "hide finished" hides the currently-selected resource, fall back to
  // the worktree's merged log view rather than stranding the user on a hidden
  // container's logs.
  useEffect(() => {
    if (!hideDone || target?.kind !== "resource") return;
    const ws = stacks.find((s) => s.worktree === target.worktree);
    const res = ws?.resources.find((r) => r.name === target.resource);
    if (res && DONE.has(res.phase)) {
      setTarget({ kind: "worktree", worktree: target.worktree });
    }
  }, [hideDone, target, stacks]);

  const lines = useMemo(() => {
    if (!target) return [];
    return target.kind === "worktree"
      ? linesForWorktree(target.worktree)
      : linesFor(target.worktree, target.resource);
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [target, linesFor, linesForWorktree, version]);

  const header = useMemo(() => describeTarget(target, stacks), [target, stacks]);

  // A previewable target is a single resource flagged `preview` with a port —
  // its running web app can be embedded instead of its log stream.
  const previewUrl = useMemo(() => {
    if (target?.kind !== "resource") return null;
    const ws = stacks.find((s) => s.worktree === target.worktree);
    const res = ws?.resources.find((r) => r.name === target.resource);
    return res?.preview && res.port ? `http://localhost:${res.port}` : null;
  }, [target, stacks]);

  return (
    <div className="app">
      <header className="masthead">
        <span className="brand">
          <Mark size={26} />
          <span className="wordmark">pando</span>
        </span>
        <span className="tagline">branch away</span>
        <span
          className={`conn ${connected ? "live" : ""}`}
          title={connected ? "live log stream connected" : "reconnecting to daemon…"}
        >
          <span className="dot" />
          {connected ? "live" : "offline"}
        </span>
        <button className="help-btn" onClick={() => setShowHelp(true)} title="keyboard shortcuts (?)">
          ?
        </button>
      </header>

      <div className="layout">
        <Sidebar
          searchRef={searchRef}
          stacks={stacks}
          target={target}
          filter={filter}
          hideDone={hideDone}
          onFilter={setFilter}
          onToggleHideDone={() => setHideDone((v) => !v)}
          onSelect={setTarget}
          onUp={(wt) => api.up(wt)}
          onDown={(wt) => api.down(wt)}
          onRestart={(wt, r) => api.restart(wt, r)}
        />

        <main className="canopy">
          <div className="canopy-head">
            <div className="canopy-title-wrap">
              <span className="canopy-title">{header.title}</span>
              <span className="canopy-sub">{header.sub}</span>
            </div>
            <div className="canopy-tools">
              {previewUrl ? (
                <div className="view-toggle">
                  <button className={showPreview ? "on" : ""} onClick={() => setShowPreview(true)}>
                    preview
                  </button>
                  <button className={!showPreview ? "on" : ""} onClick={() => setShowPreview(false)}>
                    logs
                  </button>
                </div>
              ) : null}
              <button
                className={`snap-btn ${snap ? "on" : ""}`}
                onClick={() => setSnap((v) => !v)}
                title="snap to newest logs"
              >
                <span className="snap-icon">↧</span>
                snap
              </button>
              <div className="logsearch">
                <input
                  ref={logSearchRef}
                  className="logsearch-input"
                  placeholder="search logs…"
                  value={logQuery}
                  onChange={(e) => setLogQuery(e.target.value)}
                  spellCheck={false}
                />
                {logQuery ? (
                  <button className="logsearch-clear" onClick={() => setLogQuery("")}>
                    ×
                  </button>
                ) : (
                  <kbd className="logsearch-hint">⌘L</kbd>
                )}
              </div>
            </div>
          </div>
          {target ? (
            previewUrl && showPreview ? (
              <Preview url={previewUrl} />
            ) : (
              <LogView
                lines={lines}
                query={logQuery}
                showResource={target.kind === "worktree"}
                version={version}
                snap={snap}
                onSnapChange={setSnap}
              />
            )
          ) : (
            <div className="empty">
              <Tree />
              <div className="leaf">the grove is quiet</div>
              <div>start the daemon and bring a worktree up</div>
            </div>
          )}
        </main>
      </div>

      {showHelp ? <Shortcuts onClose={() => setShowHelp(false)} /> : null}
    </div>
  );
}

function phaseReady(phase: Phase): boolean {
  return phase === "healthy" || phase === "running" || phase === "done" || phase === "skipped";
}

function firstResource(stacks: WorktreeStatus[]): Target | null {
  for (const ws of stacks) {
    if (ws.resources.length > 0) {
      return { kind: "resource", worktree: ws.worktree, resource: ws.resources[0].name };
    }
  }
  return null;
}

function describeTarget(target: Target | null, stacks: WorktreeStatus[]): { title: string; sub: string } {
  if (!target) return { title: "logs", sub: "" };
  if (target.kind === "worktree") {
    return { title: target.worktree, sub: "all resources" };
  }
  const ws = stacks.find((s) => s.worktree === target.worktree);
  const phase = ws?.resources.find((r) => r.name === target.resource)?.phase ?? "";
  return { title: target.resource, sub: `${target.worktree} · ${phaseLabel(phase)}` };
}
