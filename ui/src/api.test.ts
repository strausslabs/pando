import { test, expect, describe, afterEach } from "bun:test";
import { api } from "./api";

type FetchCall = { url: string; init?: RequestInit };

function stubFetch(impl: (url: string, init?: RequestInit) => Response): FetchCall[] {
  const calls: FetchCall[] = [];
  globalThis.fetch = ((url: string, init?: RequestInit) => {
    calls.push({ url, init });
    return Promise.resolve(impl(url, init));
  }) as typeof fetch;
  return calls;
}

const savedFetch = globalThis.fetch;
afterEach(() => {
  globalThis.fetch = savedFetch;
});

function json(body: unknown, status = 200): Response {
  return new Response(JSON.stringify(body), {
    status,
    headers: { "Content-Type": "application/json" },
  });
}

describe("api", () => {
  test("status GETs /status and parses the body", async () => {
    const calls = stubFetch(() => json([{ worktree: "main", resources: [] }]));
    const got = await api.status();
    expect(calls[0].url).toBe("/status");
    expect(got[0].worktree).toBe("main");
  });

  test("logs encodes worktree/resource and forwards tail", async () => {
    const calls = stubFetch(() => json([]));
    await api.logs("feat/x", "api:1", 50);
    const url = calls[0].url;
    expect(url).toContain("worktree=feat%2Fx");
    expect(url).toContain("resource=api%3A1");
    expect(url).toContain("tail=50");
  });

  test("up POSTs JSON with the worktree and force flag", async () => {
    const calls = stubFetch(() => json({ ok: true }));
    await api.up("wt", true);
    expect(calls[0].url).toBe("/up");
    expect(calls[0].init?.method).toBe("POST");
    expect((calls[0].init?.headers as Record<string, string>)["Content-Type"]).toBe("application/json");
    expect(JSON.parse(calls[0].init?.body as string)).toEqual({ worktree: "wt", force: true });
  });

  test("post surfaces the server error message", async () => {
    stubFetch(() => json({ error: "boom" }, 500));
    await expect(api.up("wt")).rejects.toThrow("boom");
  });

  test("post falls back to statusText when the error body is not JSON", async () => {
    stubFetch(() => new Response("nope", { status: 503, statusText: "Service Unavailable" }));
    await expect(api.down("wt")).rejects.toThrow();
  });

  test("get throws on a non-ok status", async () => {
    stubFetch(() => new Response("", { status: 500 }));
    await expect(api.status()).rejects.toThrow("request failed: 500");
  });

  test("resource actions hit their routes", async () => {
    const calls = stubFetch(() => json({ ok: true }));
    await api.restart("wt", "api");
    await api.rebuild("wt", "api");
    await api.trigger("wt", "api");
    expect(calls.map((c) => c.url)).toEqual(["/restart", "/rebuild", "/trigger"]);
  });
});
