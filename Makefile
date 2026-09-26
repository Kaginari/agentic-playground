# Build both binaries from their engines into bin/, and put them on PATH via ~/.local/bin links
# (the links point at bin/, so every `make` updates what the shell runs).
GO      ?= $(shell test -x $(HOME)/.local/go-current/bin/go && echo $(HOME)/.local/go-current/bin/go || echo go)
VERSION ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo dev)
COMMIT  := $(shell git rev-parse --short HEAD 2>/dev/null || echo none)
DATE    := $(shell date -u +%Y-%m-%dT%H:%M:%SZ)
LDFLAGS := -s -w -X main.version=$(VERSION) -X main.commit=$(COMMIT) -X main.date=$(DATE)
BIN_DIR ?= $(HOME)/.local/bin

.PHONY: all build isekai agent-one test install uninstall
all: build
build: isekai agent-one

isekai:
	cd isekai && CGO_ENABLED=0 $(GO) build -trimpath -ldflags "$(LDFLAGS)" -o ../bin/isekai ./cmd/isekai

agent-one:
	cd agent-one/engine && CGO_ENABLED=0 $(GO) build -trimpath -ldflags "$(LDFLAGS)" -o ../../bin/agent-one ./cmd/agent-one

test:
	cd isekai && $(GO) vet ./... && $(GO) test ./...
	cd agent-one/engine && $(GO) vet ./... && $(GO) test ./...

install: build
	mkdir -p $(BIN_DIR)
	ln -sf $(CURDIR)/bin/isekai $(BIN_DIR)/isekai
	ln -sf $(CURDIR)/bin/agent-one $(BIN_DIR)/agent-one
	@echo "linked into $(BIN_DIR):"; $(BIN_DIR)/isekai version; $(BIN_DIR)/agent-one version

uninstall:
	rm -f $(BIN_DIR)/isekai $(BIN_DIR)/agent-one
