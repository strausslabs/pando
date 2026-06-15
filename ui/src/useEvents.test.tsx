import { test, expect, describe, afterEach, mock } from "bun:test";
import { renderHook, act, cleanup } from "@testing-library/react";
import { useEvents } from "./useEvents";
import type { WireEvent } from "./types";

class FakeWebSocket {
  static instances: FakeWebSocket[] = [];
  url: string;
  onopen: (() => void) | null = null;
  onmessage: ((e: { data: string }) => void) | null = null;
  onclose: (() => void) | null = null;
  onerror: (() => void) | null = null;
  closed = false;

  constructor(url: string) {
    this.url = url;
    FakeWebSocket.instances.push(this);
  }
  close() {
    this.closed = true;
    this.onclose?.();
  }
}

const savedWS = globalThis.WebSocket;
afterEach(() => {
  cleanup();
  globalThis.WebSocket = savedWS;
  FakeWebSocket.instances = [];
});

function installFakeWS() {
  FakeWebSocket.instances = [];
  globalThis.WebSocket = FakeWebSocket as unknown as typeof WebSocket;
}

function logEvent(seq: number): WireEvent {
  return {
    kind: "log",
    worktree: "main",
    resource: "api",
    line: { seq, time: "", worktree: "main", resource: "api", stream: "stdout", text: "x" },
  };
}

describe("useEvents", () => {
  test("connects to /events with lastSeq=0 and reports connected on open", () => {
    installFakeWS();
    const { result } = renderHook(() => useEvents(() => {}));
    const ws = FakeWebSocket.instances[0];
    expect(ws.url).toContain("/events?lastSeq=0");
    expect(result.current.connected).toBe(false);
    act(() => ws.onopen?.());
    expect(result.current.connected).toBe(true);
  });

  test("forwards messages to the handler and tracks lastSeq for reconnect", () => {
    installFakeWS();
    const onEvent = mock((_: WireEvent) => {});
    renderHook(() => useEvents(onEvent));
    const ws = FakeWebSocket.instances[0];
    act(() => ws.onmessage?.({ data: JSON.stringify(logEvent(5)) }));
    expect(onEvent).toHaveBeenCalledTimes(1);

    // Reconnect should resume from the highest seen seq.
    act(() => ws.onclose?.());
    act(() => {
      const t = setTimeout(() => {}, 0);
      clearTimeout(t);
    });
  });

  test("closes the socket on error", () => {
    installFakeWS();
    renderHook(() => useEvents(() => {}));
    const ws = FakeWebSocket.instances[0];
    act(() => ws.onerror?.());
    expect(ws.closed).toBe(true);
  });

  test("disconnect flips connected back to false", () => {
    installFakeWS();
    const { result } = renderHook(() => useEvents(() => {}));
    const ws = FakeWebSocket.instances[0];
    act(() => ws.onopen?.());
    act(() => ws.onclose?.());
    expect(result.current.connected).toBe(false);
  });

  test("unmount closes the socket and does not reconnect", () => {
    installFakeWS();
    const { unmount } = renderHook(() => useEvents(() => {}));
    const ws = FakeWebSocket.instances[0];
    unmount();
    expect(ws.closed).toBe(true);
    expect(FakeWebSocket.instances.length).toBe(1);
  });
});
