interface Props {
  onClose: () => void;
}

const SHORTCUTS: { keys: string[]; label: string }[] = [
  { keys: ["⌘", "K"], label: "Search worktrees & resources" },
  { keys: ["⌘", "L"], label: "Search logs" },
  { keys: ["?"], label: "Show this help" },
  { keys: ["Esc"], label: "Close / clear" },
];

export function Shortcuts({ onClose }: Props) {
  return (
    <div className="modal-backdrop" onClick={onClose}>
      <div className="modal" onClick={(e) => e.stopPropagation()} role="dialog" aria-label="keyboard shortcuts">
        <header className="modal-head">
          <span className="modal-title">keyboard shortcuts</span>
          <button className="modal-close" onClick={onClose} aria-label="close">
            ×
          </button>
        </header>
        <ul className="shortcut-list">
          {SHORTCUTS.map((s) => (
            <li key={s.label} className="shortcut-row">
              <span className="shortcut-label">{s.label}</span>
              <span className="shortcut-keys">
                {s.keys.map((k) => (
                  <kbd key={k} className="kbd">
                    {k}
                  </kbd>
                ))}
              </span>
            </li>
          ))}
        </ul>
        <footer className="modal-foot">press ? anytime</footer>
      </div>
    </div>
  );
}
