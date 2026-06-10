import { defineStack, sync, run, restart } from "../runtime";
import { db } from "./shared";

const env = process.env.PANDO_ENV ?? "dev";

export default defineStack({
  name: "demo",
  services: {
    db,
    migrate: {
      task: { cmd: "bun run migrate" },
      deps: ["db"],
      runWhen: "once",
    },
    api: {
      build: {
        context: "./api",
        target: env === "prod" ? "prod" : "dev",
        args: { AWS_PROFILE: env },
        secrets: [{ id: "zscaler_cert", src: "~/zscaler.crt" }],
      },
      compose: { ports: ["$PORT_api:8000"], dependsOn: ["db"] },
      deps: ["migrate"],
      readyWhen: { httpGet: "http://localhost:$PORT_api/health", timeout: "30s" },
      liveUpdate: [
        sync("./api/src", "/app/src"),
        run("pip install -r requirements.txt", { trigger: "requirements.txt" }),
        restart(),
      ],
    },
    frontend: {
      local: {
        cmd: "bun run dev",
        cwd: "./web",
        env: { VITE_API: "http://localhost:$PORT_api" },
      },
      readyWhen: { tcp: "localhost:$PORT_frontend" },
    },
  },
});
