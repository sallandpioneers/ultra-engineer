.PHONY: ci lint build test

# Run full CI locally (same as remote)
ci: lint build test
	@echo "✓ All CI checks passed"

lint:
	@echo "Running lint..."
	@golangci-lint run --timeout=5m

build:
	@echo "Running build..."
	@go build -v ./...

test:
	@echo "Running tests..."
	@go test -race -coverprofile=coverage.out ./...
	@go tool cover -func=coverage.out | tail -1

# Quick check (no coverage, faster)
quick:
	@golangci-lint run --timeout=5m && go build ./... && go test ./...
