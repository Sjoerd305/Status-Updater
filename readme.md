# Status Updater Go

A Go-based system status monitoring and updating service designed for embedded systems.

## Features

- Real-time system status monitoring
- Automatic updates with checksum verification
- MQTT integration for status reporting
- DNS configuration management
- Robust error handling and logging
- Support for different device types

## Configuration

The application requires a configuration file with the following settings:

- `LOG_FILE`: Path to log file
- `SLEEP_INTERVAL`: Time between status checks (in seconds)
- MQTT configuration settings
- Updater service credentials
- Device-specific settings

## Components

### Gatherer
Collects system information and device status.

### Logger
Handles application logging with different severity levels.

### MQTT Client
Manages MQTT communication for status reporting.

### Updater
Handles software updates with:
- Version checking
- Secure downloads
- Checksum verification
- DNS configuration management

### System
Provides system-level utilities and panic recovery.

## Installation

1. Clone the repository
2. Configure settings in config.json
3. Build the application:
