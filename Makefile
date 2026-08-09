BINARY_NAME=envault
GO=go
INSTALL_DIR ?= /usr/local/bin
VERSION ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo "dev")
BUILD_DATE ?= $(shell date -u '+%Y-%m-%dT%H:%M:%SZ')
LDFLAGS=-ldflags "-X github.com/akhshyganesh/envault/cmd.Version=$(VERSION) -X github.com/akhshyganesh/envault/cmd.BuildDate=$(BUILD_DATE)"

.PHONY: build install uninstall clean test run

build:
	$(GO) build $(LDFLAGS) -o $(BINARY_NAME) .

# `install -d` first: /usr/local/bin does not exist on a stock macOS, and copying
# into a missing directory fails with a bare "No such file or directory". Judge
# writability by the nearest existing ancestor and elevate only when we must, so
# `make install INSTALL_DIR=$$HOME/.local/bin` doesn't leave root-owned files.
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
	  $$SUDO rm -f "$$dir/$(BINARY_NAME)" && \
	  echo "Removed $$dir/$(BINARY_NAME)"'

clean:
	rm -f $(BINARY_NAME)

test:
	$(GO) test ./...

run: build
	./$(BINARY_NAME) scan .
