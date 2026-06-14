import { useCallback, useEffect, useMemo, useRef, useState } from "react";
import type { LogLine } from "./types";
import { LogBuffers } from "./logbuffer";

export interface LogSnapshot {
  version: number;
  linesFor: (worktree: string, resource: string) => LogLine[];
  linesForWorktree: (worktree: string) => LogLine[];
}

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

  useEffect(() => () => cancelAnimationFrame(frame.current), []);

  // The store mutates buffers in place and bumps `version` on flush; binding the
  // readers to `version` is what makes them (and their callers' memos) honest
  // dependencies rather than a lint suppression.
  const snapshot = useMemo<LogSnapshot>(
    () => ({
      version,
      linesFor: (worktree, resource) => store.current.linesFor(worktree, resource),
      linesForWorktree: (worktree) => store.current.linesForWorktree(worktree),
    }),
    [version],
  );

  return { append, snapshot };
}
