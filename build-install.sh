#!/bin/bash

# Set architecture
export GOARCH=amd64
export GOOS=linux

# Build the uninstaller with static linking
cd /opt/status-updater-go/uninstaller
CGO_ENABLED=0 
go build -o uninstaller /opt/status-updater-go/uninstaller/main.go

# Build the installer with static linking
cd /opt/status-updater-go/installer
CGO_ENABLED=0 
go build -o installer /opt/status-updater-go/installer/main.go

# Unset architecture
unset GOARCH
unset GOOS