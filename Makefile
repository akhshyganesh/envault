BINARY_NAME=envault
GO=go
VERSION ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo "dev")
BUILD_DATE ?= $(shell date -u '+%Y-%m-%dT%H:%M:%SZ')
LDFLAGS=-ldflags "-X github.com/akhshyganesh/envault/cmd.Version=$(VERSION) -X github.com/akhshyganesh/envault/cmd.BuildDate=$(BUILD_DATE)"

.PHONY: build install clean test

build:
	$(GO) build $(LDFLAGS) -o $(BINARY_NAME) .

install: build
	sudo cp $(BINARY_NAME) /usr/local/bin/$(BINARY_NAME)
	@echo "✓ Installed $(BINARY_NAME) to /usr/local/bin/"

clean:
	rm -f $(BINARY_NAME)

test:
	$(GO) test ./...

run: build
	./$(BINARY_NAME) scan .
