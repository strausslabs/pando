import { useEffect } from "react";

export interface Hotkeys {
  onCommandK: () => void;
  onCommandL: () => void;
  onHelp: () => void;
  onEscape: () => void;
}

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
