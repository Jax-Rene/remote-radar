.PHONY: tidy build test run run-once clear

MODULE=remote-radar
DB_FILE ?= jobs.db

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

# Run a single crawl and exit
run-once: tidy
	go run ./cmd/server.go -once

# Clear sqlite data file
clear:
	@if [ -f "$(DB_FILE)" ]; then rm -f "$(DB_FILE)"; echo "Removed $(DB_FILE)"; else echo "No database file found at $(DB_FILE)"; fi
