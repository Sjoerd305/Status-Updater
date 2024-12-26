package logger

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"status-updater-go/config"
	"time"
)

func LogMessage(level string, message string) {
	logFile := config.Current.Log.File
	if logFile == "" {
		fmt.Printf("ERROR: LOG_FILE is not set in the configuration\n")
		return
	}

	configuredLevel := config.Current.Log.Level
	if configuredLevel == "" {
		configuredLevel = "INFO"
	}

	if config.LogLevels[level] < config.LogLevels[configuredLevel] {
		return
	}

	logEntry := fmt.Sprintf("%s [%s] %s\n", time.Now().UTC().Format(time.RFC3339), level, message)

	// Add stack trace for ERROR level logs
	if level == "ERROR" {
		stack := make([]byte, 4096)
		runtime.Stack(stack, false)
		logEntry += fmt.Sprintf("\nStack Trace:\n%s", stack)
	}

	// Ensure the directory exists
	logDir := filepath.Dir(logFile)
	if err := os.MkdirAll(logDir, 0755); err != nil {
		fmt.Printf("ERROR: Unable to create log directory %s: %v\n", logDir, err)
		return
	}

	// Open the file in append mode, create it if it doesn't exist
	file, err := os.OpenFile(logFile, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		fmt.Printf("ERROR: Unable to open or create log file %s: %v\n", logFile, err)
		return
	}
	defer file.Close()

	// Write to the file
	if _, err := file.WriteString(logEntry); err != nil {
		fmt.Printf("ERROR: Unable to write to log file %s: %v\n", logFile, err)
		return
	}

}
