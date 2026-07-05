#!/bin/bash
set -e

# Make sure we have the dependencies resolved
echo "Fetching Go dependencies..."
go mod tidy

# Build native Apple Silicon binary (linking dynamically/statically with libusb)
echo "Building native scan-go binary for Apple Silicon (darwin/arm64)..."
CGO_ENABLED=1 GOOS=darwin GOARCH=arm64 go build -o scan-go

echo "Build complete. scan-go binary details:"
file scan-go
