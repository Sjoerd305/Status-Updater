#!/bin/bash

# Set architecture
export GOARCH=amd64
export GOOS=linux

# Build the installer with static linking
cd /opt/status-updater/installer
CGO_ENABLED=0 
go build -ldflags="-w -s -extldflags=-static" -o installer /opt/status-updater/installer/main.go

# Unset architecture
unset GOARCH
unset GOOS