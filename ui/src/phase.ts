import type { Phase } from "./types";

export function phaseColor(phase: Phase): string {
  switch (phase) {
    case "healthy":
    case "running":
    case "done":
      return "var(--living)";
    case "starting":
    case "waiting":
      return "var(--pulse)";
    case "shuttingDown":
    case "liveUpdating":
      return "var(--building)";
    case "failed":
      return "var(--wither)";
    case "blocked":
      return "var(--wither)";
    case "skipped":
    case "stopped":
    case "pending":
    case "":
    default:
      return "var(--dormant)";
  }
}

export function phaseLabel(phase: Phase): string {
  if (phase === "") return "idle";
  return phase.replace(/([a-z])([A-Z])/g, "$1 $2").toLowerCase();
}
