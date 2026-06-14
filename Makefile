.PHONY: all build dist test race vet fmt lint ui ui-install ui-test ui-dev clean install tidy

BIN := bin/pando
PKG := $(shell go list ./... | grep -v /ui/node_modules/)
VERSION := $(shell git describe --tags --always --dirty 2>/dev/null || echo dev)
LDFLAGS := -X main.version=$(VERSION)

all: fmt vet test build

# Full release build: compile the UI, then embed it into the single binary.
dist: ui build

build:
	go build -ldflags "$(LDFLAGS)" -o $(BIN) ./cmd/pando

# Build the web UI into internal/web/dist for go:embed.
ui: ui-install
	cd ui && bun run build

ui-dev:
	cd ui && bun run dev

install:
	go install -ldflags "$(LDFLAGS)" ./cmd/pando

test:
	go test -count=1 $(PKG)

race:
	go test -race -count=1 $(PKG)

vet:
	go vet $(PKG)

fmt:
	gofmt -w .

fmt-check:
	@unformatted=$$(gofmt -l .); \
	if [ -n "$$unformatted" ]; then \
		echo "Not formatted:"; echo "$$unformatted"; exit 1; \
	fi

lint:
	golangci-lint run

tidy:
	go mod tidy

ui-install:
	cd ui && bun install

ui-test:
	cd ui && bun test

clean:
	rm -rf bin
