import { test, expect } from "bun:test";
import { render, screen } from "@testing-library/react";

test("DOM renders a React element", () => {
  render(<div>hello grove</div>);
  expect(screen.getByText("hello grove")).toBeDefined();
});
