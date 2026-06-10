import type { Phase } from "./types";

// Maps a resource phase to its living color on the grove.
export function phaseColor(phase: Phase): string {
  switch (phase) {
    case "healthy":
    case "running":
    case "done":
      return "var(--living)";
    case "starting":
    case "waiting":
      return "var(--pulse)";
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
  return phase === "" ? "idle" : phase;
}
