import { test, expect, describe, afterEach, mock } from "bun:test";
import { render, screen, cleanup, fireEvent } from "@testing-library/react";
import { Sidebar } from "./Sidebar";
import { bufferKey } from "./logbuffer";
import type { ResourceStatus, WorktreeStatus } from "./types";

afterEach(cleanup);

function res(over: Partial<ResourceStatus> & { name: string }): ResourceStatus {
  return { kind: "local", phase: "healthy", ready: true, ...over };
}

function stack(over: Partial<WorktreeStatus> & { worktree: string }): WorktreeStatus {
  return { branch: over.worktree, head: "", resources: [], ...over };
}

type Props = Parameters<typeof Sidebar>[0];

function renderSidebar(over: Partial<Props> = {}) {
  const props: Props = {
    stacks: [],
    target: null,
    filter: "",
    hideDone: false,
    onFilter: mock(),
    onToggleHideDone: mock(),
    onSelect: mock(),
    onUp: mock(),
    onDown: mock(),
    onRestart: mock(),
    ...over,
  };
  return { props, ...render(<Sidebar {...props} />) };
}

describe("Sidebar branch + head", () => {
  test("shows the branch name and short head hash", () => {
    renderSidebar({
      stacks: [stack({ worktree: "main", branch: "main", head: "abcdef1234567890" })],
    });
    expect(screen.getByText("main")).toBeDefined();
    expect(screen.getByText("abcdef1")).toBeDefined(); // first 7 of the hash
  });

  test("omits the head chip when there is no head", () => {
    renderSidebar({ stacks: [stack({ worktree: "main", head: "" })] });
    expect(document.querySelector(".stem-slug")).toBeNull();
  });
});

describe("Sidebar resource vitals", () => {
  test("renders cpu, mem and mem-limit when present", () => {
    renderSidebar({
      stacks: [
        stack({
          worktree: "main",
          resources: [res({ name: "api", cpuPercent: 12.5, memBytes: 1024 * 1024, memLimitBytes: 4 * 1024 * 1024 })],
        }),
      ],
    });
    expect(screen.getByText("13%")).toBeDefined(); // cpu
    expect(screen.getByText("1.0 MB")).toBeDefined(); // mem
    expect(screen.getByText("/ 4.0 MB")).toBeDefined(); // limit
  });

  test("omits vitals that are absent or zero", () => {
    renderSidebar({ stacks: [stack({ worktree: "main", resources: [res({ name: "api" })] })] });
    expect(document.querySelector(".leaf-cpu")).toBeNull();
    expect(document.querySelector(".leaf-mem")).toBeNull();
  });

  test("shows the periodic interval chip", () => {
    renderSidebar({
      stacks: [stack({ worktree: "main", resources: [res({ name: "sync", everySeconds: 1800 })] })],
    });
    expect(screen.getByText("⟳ 30m")).toBeDefined();
  });

  test("shows the port chip", () => {
    renderSidebar({
      stacks: [stack({ worktree: "main", resources: [res({ name: "web", port: 7421 })] })],
    });
    expect(screen.getByText(":7421")).toBeDefined();
  });
});

describe("Sidebar flash", () => {
  test("adds the flash class for a flashing key", () => {
    renderSidebar({
      stacks: [stack({ worktree: "main", resources: [res({ name: "api" })] })],
      flashing: new Set([bufferKey("main", "api")]),
    });
    expect(document.querySelector(".leaf-row.flash")).not.toBeNull();
  });

  test("does not flash a non-flashing resource", () => {
    renderSidebar({
      stacks: [stack({ worktree: "main", resources: [res({ name: "api" })] })],
      flashing: new Set([bufferKey("main", "other")]),
    });
    expect(document.querySelector(".leaf-row.flash")).toBeNull();
  });
});

describe("Sidebar filtering", () => {
  test("filters resources by name", () => {
    renderSidebar({
      filter: "web",
      stacks: [
        stack({ worktree: "main", resources: [res({ name: "api" }), res({ name: "webserver" })] }),
      ],
    });
    expect(screen.getByText("webserver")).toBeDefined();
    expect(screen.queryByText("api")).toBeNull();
  });

  test("a worktree-name match shows all its resources", () => {
    renderSidebar({
      filter: "main",
      stacks: [stack({ worktree: "main", resources: [res({ name: "api" }), res({ name: "web" })] })],
    });
    expect(screen.getByText("api")).toBeDefined();
    expect(screen.getByText("web")).toBeDefined();
  });

  test("reports no matches", () => {
    renderSidebar({
      filter: "nothing",
      stacks: [stack({ worktree: "main", resources: [res({ name: "api" })] })],
    });
    expect(screen.getByText("no matches")).toBeDefined();
  });
});

describe("Sidebar hide finished", () => {
  test("hides done/skipped/stopped resources and counts them", () => {
    renderSidebar({
      hideDone: true,
      stacks: [
        stack({
          worktree: "main",
          resources: [res({ name: "api", phase: "healthy" }), res({ name: "migrate", phase: "done" })],
        }),
      ],
    });
    expect(screen.getByText("api")).toBeDefined();
    expect(screen.queryByText("migrate")).toBeNull();
    expect(screen.getByText("1")).toBeDefined(); // hidden count badge
  });
});

describe("Sidebar actions", () => {
  test("up / down / restart fire their callbacks", () => {
    const { props } = renderSidebar({
      stacks: [stack({ worktree: "main", resources: [res({ name: "api" })] })],
    });
    fireEvent.click(screen.getByText("up"));
    expect(props.onUp).toHaveBeenCalledWith("main");
    fireEvent.click(screen.getByText("down"));
    expect(props.onDown).toHaveBeenCalledWith("main");
    fireEvent.click(screen.getByTitle("restart"));
    expect(props.onRestart).toHaveBeenCalledWith("main", "api");
  });

  test("clicking a resource selects it; restart does not also select", () => {
    const { props } = renderSidebar({
      stacks: [stack({ worktree: "main", resources: [res({ name: "api" })] })],
    });
    fireEvent.click(screen.getByText("api"));
    expect(props.onSelect).toHaveBeenCalledWith({ kind: "resource", worktree: "main", resource: "api" });

    (props.onSelect as ReturnType<typeof mock>).mockClear();
    fireEvent.click(screen.getByTitle("restart"));
    expect(props.onSelect).not.toHaveBeenCalled(); // stopPropagation guards the row click
  });

  test("clicking the branch selects the whole-worktree view", () => {
    const { props } = renderSidebar({
      stacks: [stack({ worktree: "main", branch: "main", resources: [res({ name: "api" })] })],
    });
    fireEvent.click(screen.getByTitle("show all logs from this worktree"));
    expect(props.onSelect).toHaveBeenCalledWith({ kind: "worktree", worktree: "main" });
  });
});
