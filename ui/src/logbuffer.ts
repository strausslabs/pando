import type { LogLine } from "./types";

export const CAP = 5000;

// NUL separator: cannot appear in worktree/resource names, so worktree-prefix matching stays unambiguous.
const SEP = "\0";

export function bufferKey(worktree: string, resource: string): string {
  return `${worktree}${SEP}${resource}`;
}

export class LogBuffers {
  private buffers = new Map<string, LogLine[]>();
  private seen = new Map<string, Set<number>>();

  append(line: LogLine): boolean {
    const k = bufferKey(line.worktree, line.resource);
    let ids = this.seen.get(k);
    if (!ids) {
      ids = new Set();
      this.seen.set(k, ids);
    }
    if (ids.has(line.seq)) return false;
    ids.add(line.seq);

    const buf = this.buffers.get(k) ?? [];
    if (buf.length === 0 || line.seq > buf[buf.length - 1].seq) {
      buf.push(line);
    } else {
      buf.splice(lowerBound(buf, line.seq), 0, line);
    }
    if (buf.length > CAP) {
      const dropped = buf.splice(0, buf.length - CAP);
      for (const d of dropped) ids.delete(d.seq);
    }
    this.buffers.set(k, buf);
    return true;
  }

  linesFor(worktree: string, resource: string): LogLine[] {
    return this.buffers.get(bufferKey(worktree, resource)) ?? [];
  }

  linesForWorktree(worktree: string): LogLine[] {
    const prefix = `${worktree}${SEP}`;
    const merged: LogLine[] = [];
    for (const [k, buf] of this.buffers) {
      if (k.startsWith(prefix)) merged.push(...buf);
    }
    merged.sort((a, b) => a.seq - b.seq);
    return merged;
  }
}

function lowerBound(buf: LogLine[], seq: number): number {
  let lo = 0;
  let hi = buf.length;
  while (lo < hi) {
    const mid = (lo + hi) >> 1;
    if (buf[mid].seq < seq) lo = mid + 1;
    else hi = mid;
  }
  return lo;
}
