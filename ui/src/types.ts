// Mirrors internal/api/api.go. Kept in sync by hand; the daemon is the source
// of truth for the wire shapes.

export type Phase =
  | "pending"
  | "waiting"
  | "starting"
  | "healthy"
  | "running"
  | "done"
  | "failed"
  | "skipped"
  | "blocked"
  | "stopped"
  | "shuttingDown"
  | "liveUpdating"
  | "";

export interface ResourceStatus {
  name: string;
  kind: string;
  phase: Phase;
  ready: boolean;
  preview?: boolean;
  port?: number;
  error?: string;
  memBytes?: number;
  memLimitBytes?: number;
  memSuggestBytes?: number;
  cpuPercent?: number;
  everySeconds?: number;
  nextRunUnix?: number;
}

export interface WorktreeStatus {
  worktree: string;
  branch: string;
  head: string;
  resources: ResourceStatus[];
}

export interface WorktreeInfo {
  path: string;
  branch: string;
  head: string;
  slug: string;
  ports: Record<string, number>;
}

export interface LogLine {
  seq: number;
  time: string;
  worktree: string;
  resource: string;
  stream: "stdout" | "stderr" | "system";
  text: string;
}

export interface WireEvent {
  kind: "log" | "phase";
  worktree: string;
  resource: string;
  phase?: Phase;
  line?: LogLine;
}
