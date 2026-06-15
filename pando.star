define_stack(
    name = "pando",
    services = {
        "go-build": service(
            task = task(cmd = "go build ./..."),
            runWhen = "onChange",
            onChange = ["**/*.go", "go.mod", "go.sum"],
            ignore = ["**/*_test.go"],
        ),
        "ui-build": service(
            task = task(cmd = "bun install && bun run build", cwd = "./ui"),
            runWhen = "onChange",
            onChange = ["ui/src/**/*.ts", "ui/src/**/*.tsx", "ui/index.html"],
            ignore = ["**/*.test.ts", "**/*.test.tsx"],
        ),
    },
)
