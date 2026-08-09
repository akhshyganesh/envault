BINARY_NAME = envault
GO          = go
INSTALL_DIR ?= /usr/local/bin

VERSION    ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo dev)
BUILD_DATE ?= $(shell date -u '+%Y-%m-%dT%H:%M:%SZ')
MODULE      = github.com/akhshyganesh/envault
LDFLAGS     = -ldflags "-X $(MODULE)/cmd.Version=$(VERSION) -X $(MODULE)/cmd.BuildDate=$(BUILD_DATE)"

.PHONY: build install uninstall test check fmt vet clean run

build:
	$(GO) build $(LDFLAGS) -o $(BINARY_NAME) .

# `install -d` first: a stock macOS has no /usr/local/bin, and copying into a
# directory that does not exist fails with a bare "No such file or directory".
# Writability is judged by the nearest existing ancestor, and sudo is used only
# when it is actually needed — so INSTALL_DIR=$$HOME/.local/bin leaves no
# root-owned files behind.
install: build
	@sh -c 'dir="$(INSTALL_DIR)"; probe="$$dir"; \
	  while [ ! -d "$$probe" ]; do probe="$$(dirname "$$probe")"; done; \
	  if [ -w "$$probe" ]; then SUDO=""; else SUDO="sudo"; echo "Installing to $$dir (requires sudo)..."; fi; \
	  $$SUDO install -d -m 755 "$$dir" && \
	  $$SUDO install -m 755 $(BINARY_NAME) "$$dir/$(BINARY_NAME)" && \
	  echo "Installed $(BINARY_NAME) to $$dir/"'

uninstall:
	@sh -c 'dir="$(INSTALL_DIR)"; \
	  if [ -w "$$dir" ]; then SUDO=""; else SUDO="sudo"; fi; \
	  $$SUDO rm -f "$$dir/$(BINARY_NAME)" && echo "Removed $$dir/$(BINARY_NAME)"'

test:
	$(GO) test ./...

# What CI runs. Use it before pushing — a gofmt miss fails the build.
check: fmt vet
	$(GO) test -race ./...

fmt:
	@unformatted=$$(gofmt -l .); \
	if [ -n "$$unformatted" ]; then echo "These files need gofmt:"; echo "$$unformatted"; exit 1; fi

vet:
	$(GO) vet ./...

clean:
	rm -f $(BINARY_NAME)

run: build
	./$(BINARY_NAME) scan .
