# Pando developing Pando. The UI dev server runs on vite's own port (open it in
# the browser); build/test are gated tasks. Run: pando start
define_stack(
    name = "pando",
    services = {
        "ui": service(
            local = cmd("bun install && bun run dev", cwd = "./ui"),
            ready = http_get("http://localhost:5173", timeout = "60s"),
        ),
        "ui-test": service(
            local = cmd("bun test --watch", cwd = "./ui"),
        ),
        "go-build": service(
            task = task(cmd = "go build ./..."),
            runWhen = "always",
        ),
    },
)
