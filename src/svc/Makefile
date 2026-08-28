GO      ?= go
GOFLAGS ?=
CGO_ENABLED ?= 0
ROOT    := $(abspath ../..)
BIN     ?= $(ROOT)/bin/dcalcon
WEB     ?= ../web

.PHONY: all build web test tidy run serve vet fmt docker

all: build web

build:
	mkdir -p $(dir $(BIN))
	CGO_ENABLED=$(CGO_ENABLED) $(GO) build $(GOFLAGS) -o $(BIN) ./cmd/dcalcon

web:
	npm --prefix $(WEB) ci
	npm --prefix $(WEB) run build

tidy:
	$(GO) mod tidy

vet:
	$(GO) vet ./...

fmt:
	$(GO) fmt ./...

test:
	CGO_ENABLED=$(CGO_ENABLED) $(GO) test ./...

run: build
	mkdir -p $(ROOT)/data
	DCALCON_SQLITE_PATH=$(ROOT)/data/dcalcon.db \
	DCALCON_HTTP_ADDR=:8080 \
	DCALCON_PUBLIC_URL=http://localhost:8080 \
	DCALCON_ADMIN_USERNAME=admin \
	DCALCON_ADMIN_PASSWORD=dcalcon-dev-pass \
	$(BIN) serve

serve: run

docker:
	podman build -t dcalcon:core --target core $(ROOT)
	podman build -t dcalcon:web --target web $(ROOT)
	podman build -t dcalcon:caldav --target caldav $(ROOT)
	podman build -t dcalcon:carddav --target carddav $(ROOT)
	podman build -t dcalcon:api --target api $(ROOT)
	podman build -t dcalcon:worker --target worker $(ROOT)
