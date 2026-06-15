import { test, expect, describe, afterEach, mock } from "bun:test";
import { renderHook, cleanup } from "@testing-library/react";
import { useHotkeys } from "./useHotkeys";

afterEach(cleanup);

function keys() {
  return {
    onCommandK: mock(() => {}),
    onCommandL: mock(() => {}),
    onHelp: mock(() => {}),
    onEscape: mock(() => {}),
  };
}

function press(init: KeyboardEventInit, target?: EventTarget) {
  const ev = new KeyboardEvent("keydown", { ...init, bubbles: true, cancelable: true });
  if (target) {
    target.dispatchEvent(ev);
  } else {
    window.dispatchEvent(ev);
  }
  return ev;
}

describe("useHotkeys", () => {
  test("cmd+K and ctrl+K fire onCommandK and preventDefault", () => {
    const k = keys();
    renderHook(() => useHotkeys(k));
    const ev = press({ key: "k", metaKey: true });
    expect(k.onCommandK).toHaveBeenCalledTimes(1);
    expect(ev.defaultPrevented).toBe(true);
    press({ key: "K", ctrlKey: true });
    expect(k.onCommandK).toHaveBeenCalledTimes(2);
  });

  test("cmd+L fires onCommandL", () => {
    const k = keys();
    renderHook(() => useHotkeys(k));
    press({ key: "l", metaKey: true });
    expect(k.onCommandL).toHaveBeenCalledTimes(1);
  });

  test("Escape fires even while typing", () => {
    const k = keys();
    renderHook(() => useHotkeys(k));
    const input = document.createElement("input");
    document.body.appendChild(input);
    press({ key: "Escape" }, input);
    expect(k.onEscape).toHaveBeenCalledTimes(1);
    input.remove();
  });

  test("? fires onHelp when not typing", () => {
    const k = keys();
    renderHook(() => useHotkeys(k));
    const ev = press({ key: "?" });
    expect(k.onHelp).toHaveBeenCalledTimes(1);
    expect(ev.defaultPrevented).toBe(true);
  });

  test("? is ignored while typing in an input/textarea/contentEditable", () => {
    const k = keys();
    renderHook(() => useHotkeys(k));

    const input = document.createElement("input");
    const textarea = document.createElement("textarea");
    const editable = document.createElement("div");
    editable.contentEditable = "true";
    for (const el of [input, textarea, editable]) {
      document.body.appendChild(el);
      press({ key: "?" }, el);
      el.remove();
    }
    expect(k.onHelp).toHaveBeenCalledTimes(0);
  });

  test("removes its listener on unmount", () => {
    const k = keys();
    const { unmount } = renderHook(() => useHotkeys(k));
    unmount();
    press({ key: "k", metaKey: true });
    expect(k.onCommandK).toHaveBeenCalledTimes(0);
  });
});
