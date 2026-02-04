.PHONY: ci lint build test security quick

# Run full CI locally (same as remote)
ci: lint build test security
	@echo "✓ All CI checks passed"

lint:
	@echo "Running lint..."
	@golangci-lint run --timeout=5m

build:
	@echo "Running build..."
	@go build ./...
	@echo "Checking go mod tidy..."
	@go mod tidy && git diff --exit-code go.mod go.sum || (echo "Run 'go mod tidy' and commit" && exit 1)

test:
	@echo "Running tests..."
	@go test -race -coverprofile=coverage.out ./...
	@COVERAGE=$$(go tool cover -func=coverage.out | grep total | awk '{print $$3}' | tr -d '%'); \
	echo "Coverage: $${COVERAGE}%"; \
	if [ $$(echo "$${COVERAGE} < 10" | bc -l) -eq 1 ]; then \
		echo "Coverage below 10% threshold"; exit 1; \
	fi

security:
	@echo "Running security checks..."
	@govulncheck ./... 2>/dev/null || go install golang.org/x/vuln/cmd/govulncheck@latest && govulncheck ./...

# Quick check (no coverage/security, faster)
quick:
	@golangci-lint run --timeout=5m && go build ./... && go test ./...
