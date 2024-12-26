# Status-Updater

**Status-Updater** is a Go-based system monitoring and updating service designed primarily for embedded systems. It provides real-time system status reporting and supports automated updates with secure verification mechanisms.

## Features

- **Real-Time Monitoring**: Continuously tracks system and device statuses.
- **Automatic Updates**: Securely downloads and installs updates with checksum validation.
- **MQTT Integration**: Publishes system statuses via the MQTT protocol.
- **DNS Management**: Manages dynamic DNS configurations.
- **Error Handling**: Robust logging and error management to ensure smooth operations.
- **Device Support**: Configurable to accommodate various device types and settings.

## Table of Contents

1. [Getting Started](#getting-started)
2. [Configuration](#configuration)
3. [Usage](#usage)
4. [Components](#components)
5. [Installation](#installation)
6. [Dependencies](#dependencies)
7. [Contributing](#contributing)

## Getting Started

These instructions will help you set up and run the Status-Updater on your system.

### Prerequisites

- [Go](https://golang.org/doc/install) (version 1.17 or later)
- MQTT broker (e.g., Mosquitto, HiveMQ)

### Clone the Repository

```bash
$ git clone https://github.com/Sjoerd305/Status-Updater.git
$ cd Status-Updater
```

## Configuration

The application is configured using a `config.json` file. Below is a sample configuration:

```json
{
  "mqtt": {
    "broker": "MQTT_BROKER_ADDRESS",
    "broker_ip": "IP_OF_MQTT_BROKER",
    "port": 8883,
    "client_id": "status-updater",
    "username": "username",
    "password": "password"
  },
  "log": {
    "level": "INFO",
    "file": "/var/log/status-updater.log"
  },
  "sleep_interval":120,
  "updater_service": {
    "metadata_url": "https://example.com/updates/status-updater/metadata.json",
    "username": "updater_username",
    "password": "updater_password"
  } 
}
```

## Usage

### Running the Application

1. Ensure `config.json` is correctly set up.
2. Build and execute the application:

```bash
$ go build -o status-updater
$ ./status-updater
```

### Logs

The application logs events to the specified log file in `config.json`. Use the following command to view logs:

```bash
$ tail -f /var/log/status-updater.log
```

## Components

### Gatherer
Collects system and device information, preparing data for MQTT reporting.

### Logger
Handles structured logging at various severity levels.

### MQTT Client
Manages MQTT communication for publishing system statuses and receiving commands.

### Updater
Manages software updates, including:

- Fetching update metadata.
- Downloading update files.
- Validating integrity using checksum.
- Executing installation commands.

### System Utilities
Provides utilities for managing system-level operations and panic recovery.

## Installation

### Build for ARM (Embedded Systems)

Use the provided `build-arm7.sh` script:

```bash
$ ./build-arm7.sh
```

### General Build

For general builds, use:

```bash
$ go build -o status-updater
```

## Dependencies

The application relies on various OS-level dependencies to function correctly. Ensure the following utilities and tools are installed on the system:

- `ip`: Used for network configuration and management.
- `mmcli`: For interacting with ModemManager for mobile broadband management.
- `curl`: For downloading update files.
- `systemctl`: To manage system services.
- `dpkg` or equivalent package managers for installing updates.

You can install these dependencies using your system's package manager. For example, on Debian based systems:

```bash
$ sudo apt-get install iproute2 modemmanager curl
```

## Acknowledgments

- [Mosquitto](https://mosquitto.org/) for MQTT communication.
- [Go](https://golang.org/) for the programming framework.
- All contributors to this project.
