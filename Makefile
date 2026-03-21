BINARY_NAME=envault
GO=go

.PHONY: build install clean test

build:
	$(GO) build -o $(BINARY_NAME) .

install: build
	sudo cp $(BINARY_NAME) /usr/local/bin/$(BINARY_NAME)
	@echo "✓ Installed $(BINARY_NAME) to /usr/local/bin/"

clean:
	rm -f $(BINARY_NAME)

test:
	$(GO) test ./...

run: build
	./$(BINARY_NAME) scan .
