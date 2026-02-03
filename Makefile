# Build variables
BINARY_NAME := ultra-engineer
MODULE := $(shell go list -m)
VERSION := $(shell git describe --tags --always --dirty 2>/dev/null || echo "dev")
COMMIT := $(shell git rev-parse --short HEAD 2>/dev/null || echo "unknown")
BUILD_DATE := $(shell date -u +"%Y-%m-%dT%H:%M:%SZ")

# Linker flags for version injection
LDFLAGS := -ldflags "-X main.version=$(VERSION) -X main.commit=$(COMMIT) -X main.buildDate=$(BUILD_DATE)"

.PHONY: build clean test run

build:
	go build $(LDFLAGS) -o $(BINARY_NAME) ./cmd/ultra-engineer

clean:
	rm -f $(BINARY_NAME)

test:
	go test ./...

run: build
	./$(BINARY_NAME)
