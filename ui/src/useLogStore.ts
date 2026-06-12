import { useCallback, useEffect, useRef, useState } from "react";
import type { LogLine } from "./types";
import { LogBuffers } from "./logbuffer";

export function useLogStore() {
  const store = useRef(new LogBuffers());
  const dirty = useRef(false);
  const frame = useRef(0);
  const [version, setVersion] = useState(0);

  const scheduleFlush = useCallback(() => {
    if (dirty.current) return;
    dirty.current = true;
    frame.current = requestAnimationFrame(() => {
      dirty.current = false;
      setVersion((v) => v + 1);
    });
  }, []);

  const append = useCallback(
    (line: LogLine) => {
      if (store.current.append(line)) scheduleFlush();
    },
    [scheduleFlush],
  );

  const linesFor = useCallback(
    (worktree: string, resource: string): LogLine[] => store.current.linesFor(worktree, resource),
    // eslint-disable-next-line react-hooks/exhaustive-deps
    [version],
  );

  const linesForWorktree = useCallback(
    (worktree: string): LogLine[] => store.current.linesForWorktree(worktree),
    // eslint-disable-next-line react-hooks/exhaustive-deps
    [version],
  );

  useEffect(() => () => cancelAnimationFrame(frame.current), []);

  return { append, linesFor, linesForWorktree, version };
}
