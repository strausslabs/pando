import type { LogLine } from "./types";

export const CAP = 5000;

// Buffer keys join worktree+resource with a NUL — a byte that cannot appear in
// either name, so prefix matching in linesForWorktree is unambiguous. The key
// and the worktree prefix MUST share this separator.
const SEP = "\0";

export function bufferKey(worktree: string, resource: string): string {
  return `${worktree}${SEP}${resource}`;
}

// LogBuffers holds per-(worktree,resource) ring buffers of log lines, keyed by
// NUL-joined names. It is pure and DOM-free: the React hook wraps it for
// rAF-batched re-renders, but every ordering/dedup/eviction rule lives here so
// it can be unit-tested directly.
export class LogBuffers {
  private buffers = new Map<string, LogLine[]>();
  private seen = new Map<string, Set<number>>();

  // append inserts a line, deduping by seq and keeping each buffer seq-sorted.
  // Returns true if the line was new (callers schedule a flush only then).
  append(line: LogLine): boolean {
    const k = bufferKey(line.worktree, line.resource);
    let ids = this.seen.get(k);
    if (!ids) {
      ids = new Set();
      this.seen.set(k, ids);
    }
    // Dedup by sequence number: live events and REST backfill overlap and can
    // arrive in any order, so arrival order is not authoritative — only seq is.
    if (ids.has(line.seq)) return false;
    ids.add(line.seq);

    const buf = this.buffers.get(k) ?? [];
    // Fast path: in-order append (the common live case). Otherwise binary-insert
    // at the sorted position so backfilled history slots ahead of live lines.
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

  // linesForWorktree merges every resource's buffer for one worktree into a
  // single seq-ordered stream — the "all logs from this worktree" view.
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

// lowerBound returns the first index whose line.seq is >= seq.
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
