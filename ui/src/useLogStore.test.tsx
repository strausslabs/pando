import { test, expect, describe, afterEach } from "bun:test";
import { renderHook, act, cleanup } from "@testing-library/react";
import { useLogStore } from "./useLogStore";
import type { LogLine } from "./types";

afterEach(cleanup);

function line(over: Partial<LogLine> & { seq: number }): LogLine {
  return {
    seq: over.seq,
    time: "2026-06-10T00:00:00.000Z",
    worktree: over.worktree ?? "main",
    resource: over.resource ?? "api",
    stream: "stdout",
    text: over.text ?? `line ${over.seq}`,
  };
}

describe("useLogStore", () => {
  test("appended lines are readable and version advances", () => {
    const { result } = renderHook(() => useLogStore());
    const before = result.current.snapshot.version;
    act(() => {
      result.current.append(line({ seq: 1, text: "hi" }));
    });
    expect(result.current.snapshot.linesFor("main", "api").map((l) => l.text)).toEqual(["hi"]);
    expect(result.current.snapshot.version).toBeGreaterThan(before);
  });

  test("a duplicate seq does not bump the version (no wasted render)", () => {
    const { result } = renderHook(() => useLogStore());
    act(() => {
      result.current.append(line({ seq: 1 }));
    });
    const after = result.current.snapshot.version;
    act(() => {
      result.current.append(line({ seq: 1 }));
    });
    expect(result.current.snapshot.version).toBe(after);
    expect(result.current.snapshot.linesFor("main", "api")).toHaveLength(1);
  });

  test("merges a worktree's resources in seq order", () => {
    const { result } = renderHook(() => useLogStore());
    act(() => {
      result.current.append(line({ seq: 3, resource: "api" }));
      result.current.append(line({ seq: 1, resource: "web" }));
      result.current.append(line({ seq: 2, resource: "api" }));
    });
    expect(result.current.snapshot.linesForWorktree("main").map((l) => l.seq)).toEqual([1, 2, 3]);
  });
});
