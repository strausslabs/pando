import { useCallback, useEffect, useMemo, useRef, useState } from "react";
import type { Phase, WireEvent, WorktreeStatus, UpdateStatus } from "./types";
import { api } from "./api";
import { useEvents } from "./useEvents";
import { useLogStore } from "./useLogStore";
import { bufferKey } from "./logbuffer";
import { useHotkeys } from "./useHotkeys";
import { LogView } from "./LogView";
import { Sidebar } from "./Sidebar";
import { Tree, Mark } from "./Tree";
import { Shortcuts } from "./Shortcuts";
import { UpdatePill } from "./UpdatePill";
import { phaseLabel } from "./phase";

type Target =
  | { kind: "resource"; worktree: string; resource: string }
  | { kind: "worktree"; worktree: string };

const DONE = new Set<Phase>(["done", "skipped", "stopped"]);

export function App() {
  const [stacks, setStacks] = useState<WorktreeStatus[]>([]);
  const [target, setTarget] = useState<Target | null>(null);
  const [filter, setFilter] = useState("");
  const [logQuery, setLogQuery] = useState("");
  const [hideDone, setHideDone] = useState(true);
  const [snap, setSnap] = useState(true);
  const [showHelp, setShowHelp] = useState(false);
  const [update, setUpdate] = useState<UpdateStatus | null>(null);
  const [flashing, setFlashing] = useState<Set<string>>(() => new Set());
  const flashTimers = useRef<Map<string, ReturnType<typeof setTimeout>>>(new Map());
  const searchRef = useRef<HTMLInputElement>(null);
  const logSearchRef = useRef<HTMLInputElement>(null);
  const { append, snapshot } = useLogStore();

  const flash = useCallback((worktree: string, resource: string) => {
    const key = bufferKey(worktree, resource);
    setFlashing((prev) => new Set(prev).add(key));
    const timers = flashTimers.current;
    clearTimeout(timers.get(key));
    timers.set(
      key,
      setTimeout(() => {
        setFlashing((prev) => {
          const next = new Set(prev);
          next.delete(key);
          return next;
        });
        timers.delete(key);
      }, 1100),
    );
  }, []);

  useEffect(() => {
    const timers = flashTimers.current;
    return () => timers.forEach(clearTimeout);
  }, []);

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
      switch (ev.kind) {
        case "log":
          if (ev.line) append(ev.line);
          return;
        case "phase":
          if (ev.phase === undefined) return;
          if (ev.phase === "liveUpdating") flash(ev.worktree, ev.resource);
          else applyPhase(ev.worktree, ev.resource, ev.phase);
          return;
      }
    },
    [append, applyPhase, flash],
  );
  const { connected } = useEvents(onEvent);

  useEffect(() => {
    let live = true;
    const refresh = async () => {
      try {
        const st = await api.status();
        if (!live) return;
        setStacks(st);
        setTarget((cur) => cur ?? firstResource(st));
      } catch (err) {
        console.warn("status refresh failed", err);
      }
    };
    refresh();
    const t = setInterval(refresh, 4000);
    return () => {
      live = false;
      clearInterval(t);
    };
  }, []);

  useEffect(() => {
    api.version().then(setUpdate).catch(() => {});
  }, []);

  const stacksRef = useRef<WorktreeStatus[]>([]);
  useEffect(() => {
    stacksRef.current = stacks;
  }, [stacks]);

  const effectiveTarget = useMemo(() => {
    if (!hideDone || target?.kind !== "resource") return target;
    const ws = stacks.find((s) => s.worktree === target.worktree);
    const res = ws?.resources.find((r) => r.name === target.resource);
    if (res && DONE.has(res.phase)) return { kind: "worktree", worktree: target.worktree } as Target;
    return target;
  }, [hideDone, target, stacks]);

  useEffect(() => {
    if (!effectiveTarget) return;
    const fetchFor = async (worktree: string, resource: string) => {
      try {
        const logs = await api.logs(worktree, resource, 1000);
        logs.forEach(append);
      } catch (err) {
        console.warn("log backfill failed", worktree, resource, err);
      }
    };
    if (effectiveTarget.kind === "resource") {
      fetchFor(effectiveTarget.worktree, effectiveTarget.resource);
    } else {
      const ws = stacksRef.current.find((s) => s.worktree === effectiveTarget.worktree);
      ws?.resources.forEach((r) => fetchFor(effectiveTarget.worktree, r.name));
    }
  }, [effectiveTarget, append]);

  const lines = useMemo(() => {
    if (!effectiveTarget) return [];
    return effectiveTarget.kind === "worktree"
      ? snapshot.linesForWorktree(effectiveTarget.worktree)
      : snapshot.linesFor(effectiveTarget.worktree, effectiveTarget.resource);
  }, [effectiveTarget, snapshot]);

  const header = useMemo(() => describeTarget(effectiveTarget, stacks), [effectiveTarget, stacks]);

  return (
    <div className="app">
      <header className="masthead">
        <span className="brand">
          <Mark />
          <span className="brand-text">
            <span className="wordmark">pando</span>
            <span className="tagline">branch away</span>
          </span>
        </span>
        <UpdatePill update={update} />
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
          flashing={flashing}
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
          {effectiveTarget ? (
            <LogView
              lines={lines}
              query={logQuery}
              showResource={effectiveTarget.kind === "worktree"}
              version={snapshot.version}
              snap={snap}
              onSnapChange={setSnap}
            />
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
