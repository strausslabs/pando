// Human-readable byte size: base 1024, 0 decimals at >=10 of a unit, 1 below.
export function formatBytes(n: number): string {
  const units = ["B", "KB", "MB", "GB", "TB"];
  let v = n;
  let i = 0;
  while (v >= 1024 && i < units.length - 1) {
    v /= 1024;
    i++;
  }
  const digits = v >= 10 || i === 0 ? 0 : 1;
  return `${v.toFixed(digits)} ${units[i]}`;
}

// CPU share of one core: 1 decimal under 10% so quiet processes don't read 0%.
export function formatCpu(pct: number): string {
  return pct < 10 ? `${pct.toFixed(1)}%` : `${pct.toFixed(0)}%`;
}

// Compact interval label: "30s" / "5m" / "2h" / "1d".
export function formatEvery(seconds: number): string {
  if (seconds < 60) return `${Math.round(seconds)}s`;
  if (seconds < 3600) return `${Math.round(seconds / 60)}m`;
  if (seconds < 86400) return `${Math.round(seconds / 3600)}h`;
  return `${Math.round(seconds / 86400)}d`;
}
