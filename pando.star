# Pando developing Pando. The daemon injects PANDO_UI_TARGET (its own API addr)
# so the vite dev server proxies to the right per-repo daemon. Run: pando up
define_stack(
    name = "pando",
    services = {
        "ui": service(
            local = cmd("bun install && bun run dev -- --port $PORT_ui --strictPort", cwd = "./ui"),
            ready = http_get("http://localhost:$PORT_ui", timeout = "60s"),
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
