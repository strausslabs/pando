import { useEffect } from "react";

export interface Hotkeys {
  onCommandK: () => void; // focus worktree/resource search
  onCommandL: () => void; // focus log search
  onHelp: () => void; // toggle shortcuts modal
  onEscape: () => void; // dismiss
}

// useHotkeys wires the global keyboard shortcuts. ⌘K / Ctrl-K focuses search;
// "?" opens the shortcuts modal; Escape dismisses. Keystrokes are ignored while
// typing in an input so they do not hijack normal editing — except ⌘K and
// Escape, which should work everywhere.
export function useHotkeys(keys: Hotkeys) {
  useEffect(() => {
    const handler = (e: KeyboardEvent) => {
      const typing = isTyping(e.target);

      if ((e.metaKey || e.ctrlKey) && e.key.toLowerCase() === "k") {
        e.preventDefault();
        keys.onCommandK();
        return;
      }
      if ((e.metaKey || e.ctrlKey) && e.key.toLowerCase() === "l") {
        e.preventDefault();
        keys.onCommandL();
        return;
      }
      if (e.key === "Escape") {
        keys.onEscape();
        return;
      }
      if (typing) return;
      if (e.key === "?") {
        e.preventDefault();
        keys.onHelp();
      }
    };
    window.addEventListener("keydown", handler);
    return () => window.removeEventListener("keydown", handler);
  }, [keys]);
}

function isTyping(target: EventTarget | null): boolean {
  if (!(target instanceof HTMLElement)) return false;
  const tag = target.tagName;
  return tag === "INPUT" || tag === "TEXTAREA" || target.isContentEditable;
}
