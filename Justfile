
# Run all tests
test:
    go test ./...

# Run golangci-lint with --fix
fix:
    golangci-lint run --fix

# Run golangci-lint without --fix
lint:
    golangci-lint run

# Build the binary
build:
    go build -o zgod .

# Install the binary
install:
    go install .

# Remove build artifacts
clean:
    rm -f zgod zgod.exe

# Run all checks (fmt, lint, test)
check: lint test
