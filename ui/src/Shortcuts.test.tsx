import { test, expect, describe, afterEach, mock } from "bun:test";
import { render, screen, fireEvent, cleanup } from "@testing-library/react";
import { Shortcuts } from "./Shortcuts";

afterEach(cleanup);

describe("Shortcuts", () => {
  test("renders all shortcut labels", () => {
    render(<Shortcuts onClose={() => {}} />);
    expect(screen.getByText("Search worktrees & resources")).toBeDefined();
    expect(screen.getByText("Search logs")).toBeDefined();
    expect(screen.getByText("Show this help")).toBeDefined();
    expect(screen.getByText("Close / clear")).toBeDefined();
  });

  test("close button invokes onClose", () => {
    const onClose = mock(() => {});
    render(<Shortcuts onClose={onClose} />);
    fireEvent.click(screen.getByLabelText("close"));
    expect(onClose).toHaveBeenCalledTimes(1);
  });

  test("backdrop click closes, but a click inside the modal does not", () => {
    const onClose = mock(() => {});
    const { container } = render(<Shortcuts onClose={onClose} />);
    fireEvent.click(screen.getByRole("dialog"));
    expect(onClose).toHaveBeenCalledTimes(0);
    fireEvent.click(container.querySelector(".modal-backdrop")!);
    expect(onClose).toHaveBeenCalledTimes(1);
  });
});
