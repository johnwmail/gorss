.PHONY: build clean test lint

VERSION ?= vDev
COMMIT  ?= $(shell git rev-parse --short HEAD 2>/dev/null || echo sha-unknown)
BUILT   ?= $(shell date -u +%Y-%m-%dT%H:%M:%SZ)
LDFLAGS  = -s -w -X main.Version=$(VERSION) -X main.CommitHash=$(COMMIT) -X main.BuildTime=$(BUILT)

build:
	go build -o gorss -ldflags="$(LDFLAGS)" ./cmd/srv

clean:
	rm -f gorss

test:
	go test -race ./...

lint:
	golangci-lint run ./...
