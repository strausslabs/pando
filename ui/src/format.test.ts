import { test, expect, describe } from "bun:test";
import { formatBytes, formatCpu, formatEvery } from "./format";

describe("formatBytes", () => {
  test("bytes under 1KB show no decimals", () => {
    expect(formatBytes(0)).toBe("0 B");
    expect(formatBytes(512)).toBe("512 B");
  });

  test("scales through KB/MB/GB", () => {
    expect(formatBytes(1024)).toBe("1.0 KB");
    expect(formatBytes(1536)).toBe("1.5 KB");
    expect(formatBytes(1024 * 1024)).toBe("1.0 MB");
    expect(formatBytes(1024 ** 3)).toBe("1.0 GB");
  });

  test(">=10 of a unit drops the decimal", () => {
    expect(formatBytes(10 * 1024 * 1024)).toBe("10 MB");
    expect(formatBytes(25.6 * 1024 * 1024)).toBe("26 MB");
  });

  test("caps at TB rather than overflowing the unit list", () => {
    expect(formatBytes(1024 ** 5)).toBe("1024 TB");
  });
});

describe("formatCpu", () => {
  test("one decimal under 10% so quiet processes are not 0%", () => {
    expect(formatCpu(0.3)).toBe("0.3%");
    expect(formatCpu(9.9)).toBe("9.9%");
  });

  test("no decimal at or above 10%", () => {
    expect(formatCpu(10)).toBe("10%");
    expect(formatCpu(143.7)).toBe("144%");
  });
});

describe("formatEvery", () => {
  test("seconds", () => {
    expect(formatEvery(30)).toBe("30s");
  });
  test("minutes", () => {
    expect(formatEvery(300)).toBe("5m");
    expect(formatEvery(1800)).toBe("30m");
  });
  test("hours", () => {
    expect(formatEvery(7200)).toBe("2h");
  });
  test("days", () => {
    expect(formatEvery(86400)).toBe("1d");
    expect(formatEvery(172800)).toBe("2d");
  });
});
