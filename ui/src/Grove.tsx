import { memo } from "react";
import type { ResourceStatus, WorktreeStatus } from "./types";
import { phaseColor, phaseLabel } from "./phase";
import { api } from "./api";

interface NodeProps {
  res: ResourceStatus;
  selected: boolean;
  onSelect: () => void;
}

// Memoized so a node re-renders only when its own phase/port/error changes —
// a log flood elsewhere never touches it.
export const Node = memo(function Node({ res, selected, onSelect }: NodeProps) {
  const color = phaseColor(res.phase);
  return (
    <div
      className={`node ${selected ? "selected" : ""}`}
      data-state={res.phase}
      style={{ ["--phase" as string]: color }}
      onClick={onSelect}
    >
      <div className="node-top">
        <span className="node-name">{res.name}</span>
        <span className="node-kind">{res.kind}</span>
      </div>
      <div className="node-meta">
        <span className="glyph" style={{ ["--phase" as string]: color }} />
        <span>{phaseLabel(res.phase)}</span>
        {res.port ? <span className="node-port">:{res.port}</span> : null}
      </div>
      {res.error ? <div className="node-err">{res.error}</div> : null}
    </div>
  );
}, areNodesEqual);

function areNodesEqual(a: NodeProps, b: NodeProps): boolean {
  return (
    a.selected === b.selected &&
    a.res.phase === b.res.phase &&
    a.res.port === b.res.port &&
    a.res.error === b.res.error &&
    a.res.name === b.res.name
  );
}

interface StemProps {
  index: number;
  ws: WorktreeStatus;
  selected: { worktree: string; resource: string } | null;
  onSelect: (worktree: string, resource: string) => void;
}

export const Stem = memo(function Stem({ index, ws, selected, onSelect }: StemProps) {
  return (
    <section className="stem" style={{ animationDelay: `${index * 0.08}s` }}>
      <header className="stem-head">
        <span className="stem-branch">{ws.branch || ws.worktree}</span>
        <span className="stem-slug">{ws.worktree}</span>
        <div className="stem-actions">
          <button className="btn" onClick={() => api.up(ws.worktree)}>
            up
          </button>
          <button className="btn ghost" onClick={() => api.down(ws.worktree)}>
            down
          </button>
        </div>
      </header>
      <div className="nodes">
        {ws.resources.map((r) => (
          <Node
            key={r.name}
            res={r}
            selected={selected?.worktree === ws.worktree && selected?.resource === r.name}
            onSelect={() => onSelect(ws.worktree, r.name)}
          />
        ))}
      </div>
    </section>
  );
});
