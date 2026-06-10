import type { WorktreeStatus, WorktreeInfo, LogLine } from "./types";

async function post(path: string, body: unknown): Promise<void> {
  const res = await fetch(path, {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify(body),
  });
  if (!res.ok) {
    const err = await res.json().catch(() => ({ error: res.statusText }));
    throw new Error(err.error ?? `request failed: ${res.status}`);
  }
}

async function get<T>(path: string): Promise<T> {
  const res = await fetch(path);
  if (!res.ok) throw new Error(`request failed: ${res.status}`);
  return res.json() as Promise<T>;
}

export const api = {
  status: () => get<WorktreeStatus[]>("/status"),
  worktrees: () => get<WorktreeInfo[]>("/worktrees"),
  logs: (worktree: string, resource: string, tail = 200) =>
    get<LogLine[]>(`/logs?worktree=${encodeURIComponent(worktree)}&resource=${encodeURIComponent(resource)}&tail=${tail}`),
  up: (worktree: string, force = false) => post("/up", { worktree, force }),
  down: (worktree: string) => post("/down", { worktree }),
  restart: (worktree: string, resource: string) => post("/restart", { worktree, resource }),
  rebuild: (worktree: string, resource: string) => post("/rebuild", { worktree, resource }),
  trigger: (worktree: string, resource: string) => post("/trigger", { worktree, resource }),
};
