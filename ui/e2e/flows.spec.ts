import { test, expect, type Page, type Request } from "@playwright/test";
import status from "./fixtures/status.json" with { type: "json" };
import worktrees from "./fixtures/worktrees.json" with { type: "json" };
import version from "./fixtures/version.json" with { type: "json" };
import logs from "./fixtures/logs.json" with { type: "json" };
import logsFeat from "./fixtures/logs-feat.json" with { type: "json" };
import events from "./fixtures/events.json" with { type: "json" };

async function replay(page: Page): Promise<Request[]> {
  const posts: Request[] = [];
  page.on("request", (r) => {
    if (r.method() === "POST") posts.push(r);
  });
  await page.route("**/status", (r) => r.fulfill({ json: status }));
  await page.route("**/worktrees", (r) => r.fulfill({ json: worktrees }));
  await page.route("**/version", (r) => r.fulfill({ json: version }));
  await page.route("**/logs**", (r) => {
    const wt = new URL(r.request().url()).searchParams.get("worktree");
    r.fulfill({ json: wt === "feat-login" ? logsFeat : logs });
  });
  await page.route("**/up", (r) => r.fulfill({ json: { ok: true } }));
  await page.route("**/down", (r) => r.fulfill({ json: { ok: true } }));
  await page.route("**/restart", (r) => r.fulfill({ json: { ok: true } }));
  await page.routeWebSocket(/\/events/, (ws) => {
    for (const ev of events) ws.send(JSON.stringify(ev));
  });
  return posts;
}

test("shows both worktrees and the periodic resource's schedule", async ({ page }) => {
  await replay(page);
  await page.goto("/");
  await expect(page.getByRole("button", { name: "main" })).toBeVisible();
  await expect(page.getByRole("button", { name: /feat\/login/ })).toBeVisible();

  // sync is a done periodic task; reveal finished resources to see its cadence.
  await page.getByRole("button", { name: /hide finished/ }).click();
  await expect(page.getByText("sync", { exact: true })).toBeVisible();
  await expect(page.getByText(/⟳/)).toBeVisible();
});

test("selecting a worktree shows its logs; switching worktrees switches logs", async ({ page }) => {
  await replay(page);
  await page.goto("/");
  // worktrees sort by branch → feat/login is selected first.
  await expect(page.getByText("feat api up")).toBeVisible();

  await page.getByRole("button", { name: "main" }).click();
  await expect(page.getByText("api listening on 8080")).toBeVisible();
});

test("filtering the sidebar narrows the worktree list", async ({ page }) => {
  await replay(page);
  await page.goto("/");
  const filter = page.getByPlaceholder("filter worktrees & resources…");
  await filter.fill("feat");
  await expect(page.getByRole("button", { name: /feat\/login/ })).toBeVisible();
  await expect(page.getByRole("button", { name: "main" })).toHaveCount(0);
});

test("up / down / restart fire the matching backend calls", async ({ page }) => {
  const posts = await replay(page);
  await page.goto("/");
  await expect(page.getByRole("button", { name: "main" })).toBeVisible();

  await page.getByRole("button", { name: "up" }).first().click();
  await page.getByRole("button", { name: "down" }).first().click();
  await page.getByTitle("restart").first().click();

  await expect.poll(() => posts.map((p) => new URL(p.url()).pathname)).toEqual(
    expect.arrayContaining(["/up", "/down", "/restart"]),
  );
});

test("log search filters the visible lines", async ({ page }) => {
  await replay(page);
  await page.goto("/");
  await expect(page.getByText("feat api up")).toBeVisible();
  await page.getByPlaceholder("search logs…").fill("nonexistent-xyz");
  await expect(page.getByText("feat api up")).toHaveCount(0);
});

test("⌘L focuses the log search input", async ({ page }) => {
  await replay(page);
  await page.goto("/");
  await expect(page.getByText("feat api up")).toBeVisible();
  await page.keyboard.press("ControlOrMeta+l");
  await expect(page.getByPlaceholder("search logs…")).toBeFocused();
});

test("? opens and closes the keyboard shortcuts modal", async ({ page }) => {
  await replay(page);
  await page.goto("/");
  await page.keyboard.press("?");
  await expect(page.getByRole("dialog", { name: "keyboard shortcuts" })).toBeVisible();
  await page.getByLabel("close").click();
  await expect(page.getByRole("dialog")).toBeHidden();
});

test("empty grove shows the quiet state", async ({ page }) => {
  await page.route("**/status", (r) => r.fulfill({ json: [] }));
  await page.route("**/worktrees", (r) => r.fulfill({ json: [] }));
  await page.route("**/version", (r) => r.fulfill({ json: version }));
  await page.routeWebSocket(/\/events/, () => {});
  await page.goto("/");
  await expect(page.getByText("the grove is quiet")).toBeVisible();
});
