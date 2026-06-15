define_stack(
    name = "periodic-demo",
    services = {
        "sync": service(
            task = task(cmd = "aws s3 sync ./data s3://bucket"),
            every = duration("30m"),
        ),
        "web": service(
            local = cmd("bun run dev", cwd = "./web"),
        ),
        "cache": service(
            compose = compose(image = "redis:7", memory = bytes("256m")),
        ),
    },
)
