.PHONY: check fmt vet staticcheck test help

# Default target: show help
help:
	@echo "Available commands:"
	@echo "  make check       - Run all quality checks (fmt, vet, staticcheck, test)"
	@echo "  make fmt         - Format all Go files"
	@echo "  make vet         - Run go vet (standard static analysis)"
	@echo "  make staticcheck - Run staticcheck (advanced static analysis)"
	@echo "  make test        - Run all unit tests with -v"

# Run all checks (equivalent to npm run check)
check: fmt vet staticcheck test

fmt:
	go fmt ./...

vet:
	go vet ./...

staticcheck:
	@command -v staticcheck >/dev/null 2>&1 || (echo "staticcheck not found, please install with: go install honnef.co/go/tools/cmd/staticcheck@latest" && exit 1)
	staticcheck ./...

lint:
	@command -v golangci-lint >/dev/null 2>&1 || (echo "golangci-lint not found, please install with: go install github.com/golangci/golangci-lint/cmd/golangci-lint@latest" && exit 1)
	golangci-lint run ./...

test:
	go test -race -count=1 -v ./...
