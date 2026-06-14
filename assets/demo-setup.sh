#!/usr/bin/env bash
# Builds the throwaway "shop" repo with two worktrees that assets/demo.tape
# records against. Re-render the GIF with: vhs assets/demo.tape
set -euo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
BIN_DIR="${PANDO_DEMO_BIN:-/tmp/pando-demo-bin}"
REPO="${PANDO_DEMO_REPO:-/tmp/pando-grove}"
FEATURE="${REPO}-feature"

"$BIN_DIR/pando" down 2>/dev/null || true
"$BIN_DIR/pando" down -w feature-checkout 2>/dev/null || true
pkill -9 -f 'pando' 2>/dev/null || true
pkill -9 -x api 2>/dev/null || true
pkill -9 -x client.sh 2>/dev/null || true
sleep 1
rm -rf "$REPO" "$FEATURE"

mkdir -p "$BIN_DIR"
go build -o "$BIN_DIR/pando" "$ROOT/cmd/pando"

mkdir -p "$REPO/cmd/api"
cd "$REPO"
git init -q
git config user.email demo@pando.dev
git config user.name "Pando Demo"

cat > go.mod <<'EOF'
module shop

go 1.25
EOF

cat > cmd/api/main.go <<'EOF'
package main

import (
	"fmt"
	"log"
	"net/http"
	"os"
)

func main() {
	http.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprintln(w, "ok")
	})
	addr := ":" + os.Getenv("PORT")
	log.Printf("api listening on %s", addr)
	log.Fatal(http.ListenAndServe(addr, nil))
}
EOF

printf '#!/bin/sh\necho "applying migrations..."\nsleep 1\necho "schema up to date"\n' > migrate.sh
# The client talks to the api at $PORT_api — same variable the api binds, so the
# two always agree on the per-worktree port Pando allocated.
printf '#!/bin/sh\nwhile true; do printf "GET localhost:%%s/health -> " "$API_PORT"; curl -s "localhost:$API_PORT/health"; sleep 2; done\n' > client.sh
chmod +x migrate.sh client.sh

cat > pando.star <<'STAR'
define_stack(
    name = "shop",
    services = {
        "api": service(
            local = cmd("go run ./cmd/api", env = {"PORT": "$PORT_api"}),
            ready = tcp("localhost:$PORT_api", timeout = "30s"),
        ),
        "migrate": service(
            task = task(cmd = "./migrate.sh"),
            deps = ["api"],
            runWhen = "once",
        ),
        "client": service(
            local = cmd("./client.sh", env = {"API_PORT": "$PORT_api"}),
            deps = ["migrate"],
        ),
    },
)
STAR

git add -A
git commit -qm "shop: api + migrate + worker"
git worktree add -q "$FEATURE" -b feature/checkout

go build ./cmd/api
