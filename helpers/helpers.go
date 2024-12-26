package helpers

import (
	"fmt"
	"os"
	"os/exec"
	"regexp"
	"status-updater-go/config"
	"status-updater-go/logger"
	"strings"
)

// Helper function to get the version of the status-updater application
func GetUpdaterVersion() string {
	cmd := exec.Command("dpkg-query", "--showformat='${Version}'", "--show", "status-updater")
	output, err := cmd.Output()
	if err != nil {
		return "Unknown"
	}
	return strings.Trim(string(output), "'")
}

// Helper function to check if any WLAN interface has an IP address
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

// Helper function to check the status of a specific service using systemctl
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

// Helper function to check the status of a specific service in /etc/init.d/ on Buildroot
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

// Helper function to extract percentage from a string
func ExtractPercentage(input string) string {
	re := regexp.MustCompile(`\d+%`)
	match := re.FindString(input)
	if match == "" {
		return "N/A"
	}
	return strings.TrimSuffix(match, "%") // Remove the trailing percentage sign
}

// Helper function to extract fields from mmcli output
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

// Function to get the SSID of the WiFi network
func GetSSID() string {
	cmd := exec.Command("iwgetid", "-r")
	output, err := cmd.Output()
	if err != nil || strings.TrimSpace(string(output)) == "" {
		logger.LogMessage("INFO", "No SSID found or failed to get SSID")
		return "N/A"
	}
	return strings.TrimSpace(string(output))
}

// Function to check internet connectivity
func IsInternetAvailable() bool {
	_, err := exec.Command("ping", "-c", "1", "172.233.38.166").Output()
	if err != nil {
		logger.LogMessage("WARN", "Internet connection is not available")
		return false
	}
	return true
}

// Function to strip ANSI color codes from a string
func StripANSI(input string) string {
	re := regexp.MustCompile(`\x1b\[[0-9;]*m`)
	return re.ReplaceAllString(input, "")
}

// Function to get the MAC address of a specific network interface
func GetMACAddress(interfaceName string) (string, error) {
	cmd := exec.Command("cat", fmt.Sprintf("/sys/class/net/%s/address", interfaceName))
	output, err := cmd.Output()
	if err != nil {
		return "", fmt.Errorf("failed to get MAC address for %s: %v", interfaceName, err)
	}
	return strings.TrimSpace(string(output)), nil
}

// Function to resolve the broker address
func ResolveBroker() string {
	cmd := exec.Command("getent", "hosts", config.Current.MQTT.Broker)
	if err := cmd.Run(); err != nil {
		return config.Current.MQTT.BrokerIP
	}
	return config.Current.MQTT.Broker
}

// Function to check if the system is running Buildroot
func IsBuildroot() bool {
	content, err := os.ReadFile("/etc/os-release")
	if err != nil {
		return false
	}
	return strings.Contains(string(content), "Buildroot")
}
