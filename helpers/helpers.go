package helpers

import (
	"fmt"
	"os"
	"os/exec"
	"regexp"
	"status-updater/config"
	"status-updater/logger"
	"strings"
	"time"
)

// CheckSystemTime verifies system time against network time and corrects it if needed
func CheckSystemTime() bool {
	// Try to get time from HTTP time server
	cmd := exec.Command("curl", "-s", "-I", "https://www.google.com")
	output, err := cmd.Output()
	if err != nil {
		logger.LogMessage("ERROR", fmt.Sprintf("Failed to query HTTP server: %s", err))
		return false
	}

	// Parse the date header from HTTP response
	re := regexp.MustCompile(`[Dd]ate: (.+)`)
	matches := re.FindStringSubmatch(string(output))
	if len(matches) < 2 {
		logger.LogMessage("ERROR", "Failed to parse HTTP date header")
		return false
	}

	// Parse the server time
	// Remove any carriage returns from the date string before parsing
	dateStr := strings.TrimRight(matches[1], "\r")
	serverTime, err := time.Parse(time.RFC1123, dateStr)
	if err != nil {
		logger.LogMessage("ERROR", fmt.Sprintf("Failed to parse server time: %s", err))
		return false
	}

	// Calculate difference between system and server time
	systemTime := time.Now()
	offset := time.Since(serverTime).Seconds()

	if offset < -30.0 || offset > 30.0 {
		logger.LogMessage("WARN", fmt.Sprintf("System time is off by %.2f seconds, correcting...", offset))
		logger.LogMessage("INFO", fmt.Sprintf("System time: %s", systemTime.Format(time.RFC3339)))
		logger.LogMessage("INFO", fmt.Sprintf("Server time: %s", serverTime.Format(time.RFC3339)))

		// Format time string for date command
		timeStr := serverTime.Format("2006-01-02 15:04:05")

		// Set system time using date command
		cmd = exec.Command("sudo", "date", "-s", timeStr)
		if err := cmd.Run(); err != nil {
			logger.LogMessage("ERROR", fmt.Sprintf("Failed to set system time: %s", err))
			return false
		}

		logger.LogMessage("INFO", "System time corrected successfully")
		return true
	}

	return true
}

// Gets status-updater version from version file or dpkg
func GetUpdaterVersion() string {
	// Try to get version from file first
	if versionBytes, err := os.ReadFile("/opt/status-updater/version"); err == nil {
		version := strings.TrimSpace(string(versionBytes))
		if version != "" {
			return version
		}
	}

	// If file doesn't exist or is empty, try dpkg on Debian systems
	if !IsBuildroot() {
		cmd := exec.Command("dpkg-query", "--showformat='${Version}'", "--show", "status-updater")
		if output, err := cmd.Output(); err == nil {
			return strings.Trim(string(output), "'")
		}
	}

	return "Unknown"
}

// Checks if any WLAN interface has IP
func HasActiveWLANInterface() bool {
	cmd := exec.Command("ip", "-o", "-4", "addr", "list")
	output, err := cmd.Output()
	if err != nil {
		logger.LogMessage("ERROR", fmt.Sprintf("Failed to list interfaces: %s", err))
		return false
	}

	lines := strings.Split(string(output), "\n")
	for _, line := range lines {
		if strings.Contains(line, "wlan") {
			logger.LogMessage("DEBUG", fmt.Sprintf("Found active WLAN interface: %s", line))
			return true
		}
	}
	return false
}

// Checks systemctl service status
func CheckServiceStatus(serviceName string) string {
	cmd := exec.Command("systemctl", "is-active", serviceName)
	output, err := cmd.Output()
	if err == nil {
		status := strings.TrimSpace(string(output))
		return fmt.Sprintf("%s: %s", serviceName, status)
	} else {
		logger.LogMessage("WARN", fmt.Sprintf("Failed to get status for service %s: %s", serviceName, err))
	}
	return ""
}

// Checks init.d service status on Buildroot
func CheckInitDServiceStatus(serviceName string) string {
	servicePath := fmt.Sprintf("/etc/init.d/%s", serviceName)
	if _, err := os.Stat(servicePath); err == nil {
		cmd := exec.Command(servicePath, "status")
		output, err := cmd.Output()
		if err == nil {
			status := strings.TrimSpace(string(output))
			if strings.Contains(status, "running") {
				return fmt.Sprintf("%s: running", serviceName)
			} else {
				return fmt.Sprintf("%s: stopped", serviceName)
			}
		} else {
			logger.LogMessage("WARN", fmt.Sprintf("Failed to get status for service %s: %s", serviceName, err))
		}
	} else {
		logger.LogMessage("WARN", fmt.Sprintf("Service %s not found in /etc/init.d/", serviceName))
	}
	return ""
}

// Extracts percentage value from string
func ExtractPercentage(input string) string {
	re := regexp.MustCompile(`\d+%`)
	match := re.FindString(input)
	if match == "" {
		return "N/A"
	}
	return strings.TrimSuffix(match, "%")
}

// Extracts fields from mmcli output
func ExtractField(output, field string) string {
	lines := strings.Split(output, "\n")
	for _, line := range lines {
		if strings.Contains(line, field) {
			parts := strings.SplitN(line, ":", 2)
			if len(parts) == 2 {
				return strings.TrimSpace(parts[1])
			}
		}
	}
	return "unknown"
}

// Gets current WiFi SSID
func GetSSID() string {
	cmd := exec.Command("iwgetid", "-r")
	output, err := cmd.Output()
	if err != nil || strings.TrimSpace(string(output)) == "" {
		logger.LogMessage("INFO", "No SSID found or failed to get SSID")
		return "N/A"
	}
	return strings.TrimSpace(string(output))
}

// Pings test IP to check internet connectivity
func IsInternetAvailable() bool {
	_, err := exec.Command("ping", "-c", "1", "172.233.38.166").Output()
	if err != nil {
		logger.LogMessage("WARN", "Internet connection is not available")
		return false
	}
	return true
}

// Removes ANSI color codes from string
func StripANSI(input string) string {
	re := regexp.MustCompile(`\x1b\[[0-9;]*m`)
	return re.ReplaceAllString(input, "")
}

// Gets MAC address for specified interface
func GetMACAddress(interfaceName string) (string, error) {
	cmd := exec.Command("cat", fmt.Sprintf("/sys/class/net/%s/address", interfaceName))
	output, err := cmd.Output()
	if err != nil {
		return "", fmt.Errorf("failed to get MAC address for %s: %v", interfaceName, err)
	}
	return strings.TrimSpace(string(output)), nil
}

// Resolves broker address to IP if needed
func ResolveBroker() string {
	cmd := exec.Command("getent", "hosts", config.Current.MQTT.Broker)
	if err := cmd.Run(); err != nil {
		return config.Current.MQTT.BrokerIP
	}
	return config.Current.MQTT.Broker
}

// Detects if system is running Buildroot
func IsBuildroot() bool {
	content, err := os.ReadFile("/etc/os-release")
	if err != nil {
		return false
	}
	return strings.Contains(string(content), "Buildroot")
}
