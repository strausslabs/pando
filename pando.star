define_stack(
    name = "pando",
    services = {
        "go-build": service(
            task = task(cmd = "go build ./..."),
            runWhen = "onChange",
            onChange = ["**/*.go", "go.mod", "go.sum"],
        ),
    },
)
