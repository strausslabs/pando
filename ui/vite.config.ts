import { defineConfig } from "vite";
import react from "@vitejs/plugin-react";

const DAEMON = process.env.PANDO_UI_TARGET ?? "http://127.0.0.1:7420";

export default defineConfig({
  plugins: [react()],
  // Build straight into the Go web package so the daemon can go:embed it into
  // the single binary.
  build: {
    outDir: "../internal/web/dist",
    emptyOutDir: true,
  },
  server: {
    proxy: {
      "/status": DAEMON,
      "/worktrees": DAEMON,
      "/logs": DAEMON,
      "/up": DAEMON,
      "/down": DAEMON,
      "/restart": DAEMON,
      "/rebuild": DAEMON,
      "/trigger": DAEMON,
      "/exec": DAEMON,
      "/events": { target: DAEMON, ws: true },
    },
  },
});
