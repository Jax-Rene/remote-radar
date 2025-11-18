.PHONY: tidy build test run

MODULE=remote-radar

# Sync dependencies
 tidy:
	go mod tidy

# Build all binaries
 build: tidy
	go build ./...

# Run tests with coverage
 test: tidy
	go test ./... -cover

# Run the server
 run: tidy
	go run ./cmd/server.go
