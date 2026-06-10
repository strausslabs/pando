import { useEffect, useLayoutEffect, useRef, useState } from "react";
import type { LogLine } from "./types";

const ROW = 21; // px per line; fixed-height rows make windowing exact.
const OVERSCAN = 12;

// Render the daemon's RFC3339 timestamp in the viewer's local time zone, with
// millisecond precision for sub-second log ordering.
function ts(iso: string): string {
  const d = new Date(iso);
  if (isNaN(d.getTime())) return "--:--:--.---";
  const t = d.toLocaleTimeString(undefined, { hour12: false });
  const ms = String(d.getMilliseconds()).padStart(3, "0");
  return `${t}.${ms}`;
}

// LogView windows a potentially huge buffer: only the visible slice (plus a
// small overscan) is rendered to the DOM, so a 5000-line buffer paints ~40
// rows. Auto-follows the tail unless the user scrolls up.
export function LogView({ lines }: { lines: LogLine[] }) {
  const scroller = useRef<HTMLDivElement>(null);
  const [scrollTop, setScrollTop] = useState(0);
  const [height, setHeight] = useState(0);
  const follow = useRef(true);

  useLayoutEffect(() => {
    const el = scroller.current;
    if (!el) return;
    const ro = new ResizeObserver(() => setHeight(el.clientHeight));
    ro.observe(el);
    setHeight(el.clientHeight);
    return () => ro.disconnect();
  }, []);

  useEffect(() => {
    const el = scroller.current;
    if (el && follow.current) {
      el.scrollTop = el.scrollHeight;
    }
  }, [lines.length]);

  const total = lines.length * ROW;
  const start = Math.max(0, Math.floor(scrollTop / ROW) - OVERSCAN);
  const end = Math.min(lines.length, Math.ceil((scrollTop + height) / ROW) + OVERSCAN);
  const slice = lines.slice(start, end);

  return (
    <div
      ref={scroller}
      className="logstream"
      onScroll={(e) => {
        const el = e.currentTarget;
        setScrollTop(el.scrollTop);
        follow.current = el.scrollHeight - el.scrollTop - el.clientHeight < ROW * 2;
      }}
    >
      <div style={{ height: total, position: "relative" }}>
        {slice.map((l, i) => (
          <div
            key={l.seq}
            className={`logline ${l.stream}`}
            style={{ position: "absolute", top: (start + i) * ROW, left: 0, right: 0 }}
          >
            <span className="ts">{ts(l.time)}</span>
            <span className="txt">{l.text}</span>
          </div>
        ))}
      </div>
    </div>
  );
}
