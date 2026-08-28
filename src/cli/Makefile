GO      ?= go
GOFLAGS ?=
CGO_ENABLED ?= 0
ROOT    := $(abspath ../..)
BIN     ?= $(ROOT)/bin/dcalcon-cli

.PHONY: all build test tidy vet fmt run

all: build

build:
	mkdir -p $(dir $(BIN))
	CGO_ENABLED=$(CGO_ENABLED) $(GO) build $(GOFLAGS) -o $(BIN) ./cmd/dcalcon-cli

tidy:
	$(GO) mod tidy

vet:
	$(GO) vet ./...

fmt:
	$(GO) fmt ./...

test:
	CGO_ENABLED=$(CGO_ENABLED) $(GO) test ./...

run: build
	$(BIN) --help
