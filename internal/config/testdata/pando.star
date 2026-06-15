env = "dev"

def db():
    return service(compose = compose(image = "postgres:16", ports = ["$PORT_db:5432"]))

define_stack(
    name = "demo",
    services = {
        "db": db(),
        "migrate": service(
            task = task(cmd = "bun run migrate"),
            deps = ["db"],
            runWhen = "once",
        ),
        "api": service(
            build = build(
                context = "./api",
                target = "prod" if env == "prod" else "dev",
                args = {"AWS_PROFILE": env},
                secrets = [{"id": "zscaler_cert", "src": "~/zscaler.crt"}],
            ),
            compose = compose(ports = ["$PORT_api:8000"], dependsOn = ["db"]),
            deps = ["migrate"],
            ready = http_get("http://localhost:$PORT_api/health", timeout = "30s"),
            liveUpdate = [
                sync("./api/src", "/app/src"),
                run("pip install -r requirements.txt", trigger = "requirements.txt"),
                restart_container(),
            ],
        ),
        "frontend": service(
            local = cmd("bun run dev", cwd = "./web", env = {"VITE_API": "http://localhost:$PORT_api"}),
            ready = tcp("localhost:$PORT_frontend"),
        ),
    },
)
