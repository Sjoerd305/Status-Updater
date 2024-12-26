package system

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"os/signal"
	"sort"
	"strings"
	"sync"
	"syscall"
	"time"

	"status-updater/logger"
)

func MonitorNetworkChanges(ctx context.Context) {
	var lastMainInterfaces string
	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()

	// Helper function to get main interfaces (excluding VPN/tunnel interfaces)
	getMainInterfaces := func() string {
		cmd := exec.Command("ip", "-o", "-4", "addr", "list")
		output, err := cmd.Output()
		if err != nil {
			logger.LogMessage("ERROR", fmt.Sprintf("Failed to get IP addresses: %s", err))
			return ""
		}

		var interfaces []string
		lines := strings.Split(string(output), "\n")
		for _, line := range lines {
			if line != "" {
				parts := strings.Fields(line)
				if len(parts) >= 4 {
					iface := parts[1]
					// Skip VPN and tunnel interfaces
					if !strings.HasPrefix(iface, "tun") && !strings.HasPrefix(iface, "tap") {
						ip := strings.Split(parts[3], "/")[0]
						interfaces = append(interfaces, fmt.Sprintf("%s:%s", iface, ip))
					}
				}
			}
		}
		sort.Strings(interfaces)
		return strings.Join(interfaces, ",")
	}

	// Initialize last known state
	lastMainInterfaces = getMainInterfaces()

	for {
		select {
		case <-ticker.C:
			currentMainInterfaces := getMainInterfaces()
			if lastMainInterfaces != currentMainInterfaces && lastMainInterfaces != "" {
				logger.LogMessage("INFO", "Network interface change detected")
				// Instead of canceling context, just log the change
				// The next status update will use the new network configuration
			}
			lastMainInterfaces = currentMainInterfaces
		case <-ctx.Done():
			return
		}
	}
}

func HandleShutdown(cancel context.CancelFunc, wg *sync.WaitGroup) {
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, os.Interrupt, syscall.SIGTERM)

	<-sigChan
	logger.LogMessage("INFO", "Termination signal received. Initiating graceful shutdown...")
	cancel() // Cancel the context to stop the main loop

	// Wait for all goroutines to complete
	wg.Wait()

	logger.LogMessage("INFO", "Graceful shutdown complete.")
	os.Exit(0)
}

func RecoverFromPanic() {
	if r := recover(); r != nil {
		logger.LogMessage("ERROR", fmt.Sprintf("Recovered from panic: %v", r))
	}
}
