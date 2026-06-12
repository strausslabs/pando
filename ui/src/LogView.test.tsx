import { test, expect, describe, afterEach } from "bun:test";
import { render, screen, cleanup } from "@testing-library/react";
import { LogView } from "./LogView";
import type { LogLine } from "./types";

afterEach(cleanup);

function line(over: Partial<LogLine> & { seq: number }): LogLine {
  return {
    seq: over.seq,
    time: over.time ?? "2026-06-10T12:00:00.000Z",
    worktree: over.worktree ?? "main",
    resource: over.resource ?? "api",
    stream: over.stream ?? "stdout",
    text: over.text ?? `line ${over.seq}`,
  };
}

const noop = () => {};

describe("LogView empty states", () => {
  test("no lines shows 'no output yet'", () => {
    render(<LogView lines={[]} query="" showResource={false} version={0} snap onSnapChange={noop} />);
    expect(screen.getByText("no output yet")).toBeDefined();
  });

  test("a query with no matches reports it", () => {
    render(
      <LogView lines={[line({ seq: 1, text: "hello" })]} query="zzz" showResource={false} version={1} snap onSnapChange={noop} />,
    );
    expect(screen.getByText('no lines match "zzz"')).toBeDefined();
  });
});

describe("LogView rendering", () => {
  test("renders matching lines and filters out the rest", () => {
    const lines = [line({ seq: 1, text: "keep me" }), line({ seq: 2, text: "drop this" })];
    render(<LogView lines={lines} query="keep" showResource={false} version={1} snap onSnapChange={noop} />);
    const rows = document.querySelectorAll(".logline");
    expect(rows).toHaveLength(1);
    expect(rows[0].textContent).toContain("keep me"); // text spans a <mark> split
  });

  test("shows the resource column only in the merged worktree view", () => {
    const lines = [line({ seq: 1, resource: "web", text: "x" })];
    const { rerender } = render(
      <LogView lines={lines} query="" showResource={false} version={1} snap onSnapChange={noop} />,
    );
    expect(screen.queryByText("web")).toBeNull();
    rerender(<LogView lines={lines} query="" showResource version={1} snap onSnapChange={noop} />);
    expect(screen.getByText("web")).toBeDefined();
  });

  test("highlights the query match with a <mark>", () => {
    render(
      <LogView lines={[line({ seq: 1, text: "an ERROR here" })]} query="error" showResource={false} version={1} snap onSnapChange={noop} />,
    );
    const mark = document.querySelector("mark.hl");
    expect(mark?.textContent).toBe("ERROR");
  });

  test("pretty-prints a JSON line into a <pre>", () => {
    render(
      <LogView lines={[line({ seq: 1, text: '{"level":"info"}' })]} query="" showResource={false} version={1} snap onSnapChange={noop} />,
    );
    const pre = document.querySelector("pre.ln-json");
    expect(pre?.textContent).toContain('"level": "info"');
  });

  test("tags each row with its stream class", () => {
    const lines = [line({ seq: 1, stream: "stderr", text: "boom" })];
    render(<LogView lines={lines} query="" showResource={false} version={1} snap onSnapChange={noop} />);
    expect(document.querySelector(".logline.stderr")).not.toBeNull();
  });
});

describe("LogView React-key isolation (cross-resource bug)", () => {
  // The merged view can hold two lines that share a seq but belong to different
  // resources. Keys must include worktree+resource so React renders BOTH rows
  // and a search highlight does not bleed across them.
  test("two resources sharing a seq both render", () => {
    const lines = [
      line({ seq: 1, resource: "api", text: "from-api" }),
      line({ seq: 1, resource: "web", text: "from-web" }),
    ];
    render(<LogView lines={lines} query="" showResource version={1} snap onSnapChange={noop} />);
    const rows = document.querySelectorAll(".logline");
    expect(rows).toHaveLength(2);
    expect(screen.getByText("from-api")).toBeDefined();
    expect(screen.getByText("from-web")).toBeDefined();
  });

  test("highlight stays on the matching row only", () => {
    const lines = [
      line({ seq: 1, resource: "api", text: "needle here" }),
      line({ seq: 1, resource: "web", text: "haystack only" }),
    ];
    render(<LogView lines={lines} query="needle" showResource version={1} snap onSnapChange={noop} />);
    const marks = document.querySelectorAll("mark.hl");
    expect(marks).toHaveLength(1);
    expect(marks[0].textContent).toBe("needle");
  });
});

describe("LogView render cap", () => {
  // Only the most recent RENDER_CAP (800) rows mount, keeping the DOM bounded.
  test("caps mounted rows and keeps the newest", () => {
    const lines = Array.from({ length: 1000 }, (_, i) => line({ seq: i + 1 }));
    render(<LogView lines={lines} query="" showResource={false} version={1} snap onSnapChange={noop} />);
    const rows = document.querySelectorAll(".logline");
    expect(rows).toHaveLength(800);
    // newest line present, oldest dropped
    expect(screen.getByText("line 1000")).toBeDefined();
    expect(screen.queryByText("line 1")).toBeNull();
  });
});
