import { test, expect } from "bun:test";
import { phaseColor, phaseLabel } from "./phase";

test("healthy phases map to living color", () => {
  expect(phaseColor("healthy")).toBe("var(--living)");
  expect(phaseColor("running")).toBe("var(--living)");
  expect(phaseColor("done")).toBe("var(--living)");
});

test("failure phases map to wither color", () => {
  expect(phaseColor("failed")).toBe("var(--wither)");
  expect(phaseColor("blocked")).toBe("var(--wither)");
});

test("transitional phases map to pulse/building", () => {
  expect(phaseColor("starting")).toBe("var(--pulse)");
  expect(phaseColor("liveUpdating")).toBe("var(--building)");
});

test("idle and unknown map to dormant", () => {
  expect(phaseColor("")).toBe("var(--dormant)");
  expect(phaseColor("pending")).toBe("var(--dormant)");
  expect(phaseColor("stopped")).toBe("var(--dormant)");
});

test("empty phase labels as idle", () => {
  expect(phaseLabel("")).toBe("idle");
  expect(phaseLabel("healthy")).toBe("healthy");
});
