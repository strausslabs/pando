import { test, expect, describe, afterEach } from "bun:test";
import { render, screen, cleanup } from "@testing-library/react";
import { UpdatePill } from "./UpdatePill";

afterEach(cleanup);

describe("UpdatePill", () => {
  test("renders the version jump and a releases link when an update is available", () => {
    render(<UpdatePill update={{ current: "v0.1.7", latest: "v0.2.0", available: true }} />);
    const link = screen.getByRole("link");
    expect(link.textContent).toContain("v0.1.7");
    expect(link.textContent).toContain("v0.2.0");
    expect(link.getAttribute("href")).toBe("https://github.com/strausslabs/pando/releases/latest");
  });

  test("renders nothing when no update is available", () => {
    const { container } = render(
      <UpdatePill update={{ current: "v0.2.0", latest: "v0.2.0", available: false }} />,
    );
    expect(container.firstChild).toBeNull();
  });

  test("renders nothing when the version status is absent", () => {
    const { container } = render(<UpdatePill update={null} />);
    expect(container.firstChild).toBeNull();
  });
});
