import { defineStack } from "../runtime";

export default defineStack({
  name: "periodic-demo",
  services: {
    sync: {
      task: { cmd: "aws s3 sync ./data s3://bucket" },
      every: "30m",
    },
    web: {
      local: { cmd: "bun run dev", cwd: "./web" },
      preview: true,
    },
  },
});
