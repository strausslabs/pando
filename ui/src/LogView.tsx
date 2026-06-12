import { useEffect, useMemo, useRef } from "react";
import type { LogLine } from "./types";
import { tryPrettyJSON, matchesQuery } from "./logfmt";
import { bufferKey } from "./logbuffer";

function ts(iso: string): string {
  const d = new Date(iso);
  if (isNaN(d.getTime())) return "--:--:--.---";
  const t = d.toLocaleTimeString(undefined, { hour12: false });
  const ms = String(d.getMilliseconds()).padStart(3, "0");
  return `${t}.${ms}`;
}

interface Props {
  lines: LogLine[];
  query: string;
  showResource: boolean;
  version: number;
  snap: boolean;
  onSnapChange: (snap: boolean) => void;
}

const RENDER_CAP = 800;

export function LogView({ lines, query, showResource, version, snap, onSnapChange }: Props) {
  const scroller = useRef<HTMLDivElement>(null);
  const programmatic = useRef(false);

  const filtered = useMemo(() => {
    const matched = query ? lines.filter((l) => matchesQuery(l.text, query)) : lines;
    return matched.length > RENDER_CAP ? matched.slice(matched.length - RENDER_CAP) : matched;
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [lines, query, version, lines.length]);

  // Snap depends on `version` (bumped every flush), not filtered.length: past RENDER_CAP the length saturates and snap would silently die on busy logs.
  useEffect(() => {
    const el = scroller.current;
    if (el && snap) {
      programmatic.current = true;
      el.scrollTop = el.scrollHeight;
    }
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [version, filtered.length, snap]);

  if (filtered.length === 0) {
    return (
      <div className="logstream" ref={scroller}>
        <div className="log-empty">{query ? `no lines match "${query}"` : "no output yet"}</div>
      </div>
    );
  }

  return (
    <div
      ref={scroller}
      className="logstream"
      onScroll={(e) => {
        const el = e.currentTarget;
        if (programmatic.current) {
          programmatic.current = false;
          return;
        }
        const atBottom = el.scrollHeight - el.scrollTop - el.clientHeight < 48;
        if (atBottom !== snap) onSnapChange(atBottom);
      }}
    >
      {filtered.map((l) => (
        // Key on worktree+resource+seq, not seq alone: in the merged view two resources can share a seq, and a bare seq key makes React reuse the wrong row's DOM.
        <LogRow key={`${bufferKey(l.worktree, l.resource)}\0${l.seq}`} line={l} showResource={showResource} query={query} />
      ))}
    </div>
  );
}

function LogRow({ line, showResource, query }: { line: LogLine; showResource: boolean; query: string }) {
  const pretty = tryPrettyJSON(line.text);
  return (
    <div className={`logline ${line.stream}`}>
      <span className="ts">{ts(line.time)}</span>
      {showResource ? <span className="ln-res">{line.resource}</span> : null}
      {pretty ? (
        <pre className="ln-json">{highlight(pretty, query)}</pre>
      ) : (
        <span className="txt">{highlight(line.text, query)}</span>
      )}
    </div>
  );
}

function highlight(text: string, query: string): React.ReactNode {
  if (!query) return text;
  const lower = text.toLowerCase();
  const q = query.toLowerCase();
  const parts: React.ReactNode[] = [];
  let i = 0;
  let n = 0;
  while (i < text.length) {
    const hit = lower.indexOf(q, i);
    if (hit < 0) {
      parts.push(text.slice(i));
      break;
    }
    if (hit > i) parts.push(text.slice(i, hit));
    parts.push(
      <mark key={n++} className="hl">
        {text.slice(hit, hit + q.length)}
      </mark>,
    );
    i = hit + q.length;
  }
  return parts;
}
