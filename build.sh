#!/bin/bash

# Exit on any error
set -e

# Set build environment variables for complete static linking
export GOARCH=amd64
export GOOS=linux
export CGO_ENABLED=0
export GOFLAGS="-tags=netgo,osusergo"  # Force use of Go's native networking and user packages

# Create build directory if it doesn't exist
BUILD_DIR="./build"
mkdir -p $BUILD_DIR

# Build the main application with fully static linking
echo "Building status-updater..."
go build -a -ldflags='-w -s -extldflags "-static"' -tags netgo,osusergo -o $BUILD_DIR/status-updater main.go

# Build the installer with fully static linking
echo "Building installer..."
cd installer
go build -a -ldflags='-w -s -extldflags "-static"' -tags netgo,osusergo -o ../build/installer main.go
cd ..

# Set executable permissions
chmod +x $BUILD_DIR/status-updater
chmod +x $BUILD_DIR/installer

echo "Build complete! Binaries are in the $BUILD_DIR directory"

# Verify static linking
echo -e "\nVerifying static linking:"
file $BUILD_DIR/status-updater
file $BUILD_DIR/installer 