import { useCallback, useEffect, useMemo, useState } from "react";
import type { Phase, WireEvent, WorktreeStatus } from "./types";
import { api } from "./api";
import { useEvents } from "./useEvents";
import { useLogStore } from "./useLogStore";
import { LogView } from "./LogView";
import { Stem } from "./Grove";
import { phaseLabel } from "./phase";

type Selection = { worktree: string; resource: string } | null;

export function App() {
  const [stacks, setStacks] = useState<WorktreeStatus[]>([]);
  const [selected, setSelected] = useState<Selection>(null);
  const { append, linesFor, version } = useLogStore();

  // Phase updates patch the status tree in place (low frequency); the grove
  // re-renders, the log store does not.
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
      if (ev.kind === "log" && ev.line) {
        append(ev.line);
      } else if (ev.kind === "phase" && ev.phase !== undefined) {
        applyPhase(ev.worktree, ev.resource, ev.phase);
      }
    },
    [append, applyPhase],
  );

  const { connected } = useEvents(onEvent);

  const refresh = useCallback(async () => {
    try {
      const st = await api.status();
      setStacks(st);
      setSelected((cur) => cur ?? firstResource(st));
    } catch {
      /* daemon not up yet; the poll retries */
    }
  }, []);

  useEffect(() => {
    refresh();
    const t = setInterval(refresh, 4000);
    return () => clearInterval(t);
  }, [refresh]);

  const lines = useMemo(
    () => (selected ? linesFor(selected.worktree, selected.resource) : []),
    [selected, linesFor],
    // version drives re-read of the batched log buffer.
    // eslint-disable-next-line react-hooks/exhaustive-deps
  );
  void version;

  const selectedPhase = useMemo(() => {
    if (!selected) return "";
    const ws = stacks.find((s) => s.worktree === selected.worktree);
    return ws?.resources.find((r) => r.name === selected.resource)?.phase ?? "";
  }, [selected, stacks]);

  return (
    <div className="app">
      <header className="masthead">
        <span className="wordmark">
          Pand<span className="o">o</span>
        </span>
        <span className="tagline">one root, many stems</span>
        <span className={`conn ${connected ? "live" : ""}`}>
          <span className="dot" />
          {connected ? "rooted" : "severed"}
        </span>
      </header>

      <div className="grove">
        <div className="stems">
          {stacks.length === 0 ? (
            <div className="empty">
              <div className="leaf">the grove is quiet</div>
              <div>start the daemon and bring a worktree up</div>
            </div>
          ) : (
            stacks.map((ws, i) => (
              <Stem
                key={ws.worktree}
                index={i}
                ws={ws}
                selected={selected}
                onSelect={(worktree, resource) => setSelected({ worktree, resource })}
              />
            ))
          )}
        </div>

        <aside className="canopy">
          <div className="canopy-head">
            {selected ? (
              <>
                <span className="canopy-title">{selected.resource}</span>
                <span className="canopy-sub">
                  {selected.worktree} · {phaseLabel(selectedPhase)}
                </span>
              </>
            ) : (
              <span className="canopy-title">logs</span>
            )}
          </div>
          {selected ? (
            <LogView lines={lines} />
          ) : (
            <div className="empty">
              <div className="leaf">no leaf selected</div>
              <div>pick a resource to read its logs</div>
            </div>
          )}
        </aside>
      </div>
    </div>
  );
}

function phaseReady(phase: Phase): boolean {
  return phase === "healthy" || phase === "running" || phase === "done" || phase === "skipped";
}

function firstResource(stacks: WorktreeStatus[]): Selection {
  for (const ws of stacks) {
    if (ws.resources.length > 0) {
      return { worktree: ws.worktree, resource: ws.resources[0].name };
    }
  }
  return null;
}
