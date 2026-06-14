import type { UpdateStatus } from "./types";

export function UpdatePill({ update }: { update: UpdateStatus | null }) {
  if (!update?.available) return null;
  return (
    <a
      className="update-pill"
      href="https://github.com/strausslabs/pando/releases/latest"
      target="_blank"
      rel="noreferrer"
      title="a newer pando is available — run: brew upgrade pando"
    >
      {update.current} → {update.latest}
    </a>
  );
}
