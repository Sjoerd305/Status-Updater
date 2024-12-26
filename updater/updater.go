package updater

import (
	"crypto/md5"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"status-updater/config"
	"status-updater/gatherer"
	"status-updater/logger"
)

// Add this struct definition at the top of the file
type UpdateMetadata struct {
	Version  string `json:"version"`
	URL      string `json:"url"`
	Checksum string `json:"checksum"`
}

// Function to check and fix DNS configuration
func checkAndFixDNS() {
	// Check if wwan0 interface exists and is up
	cmd := exec.Command("ip", "link", "show", "wwan0")
	if err := cmd.Run(); err != nil {
		logger.LogMessage("DEBUG", "wwan0 interface not found, skipping DNS check")
		return
	}

	// Test DNS resolution
	cmd = exec.Command("timeout", "2", "getent", "hosts", "google.com")
	if err := cmd.Run(); err != nil {
		logger.LogMessage("WARN", "DNS resolution failed, attempting to fix DNS configuration")

		// Backup existing resolv.conf
		if err := exec.Command("cp", "/etc/resolv.conf", "/etc/resolv.conf.backup").Run(); err != nil {
			logger.LogMessage("ERROR", fmt.Sprintf("Failed to backup resolv.conf: %v", err))
			return
		}

		// Write new resolv.conf with Cloudflare DNS
		dnsConfig := []byte("nameserver 1.1.1.1\nnameserver 1.0.0.1\n")
		if err := os.WriteFile("/etc/resolv.conf", dnsConfig, 0644); err != nil {
			logger.LogMessage("ERROR", fmt.Sprintf("Failed to update resolv.conf: %v", err))
			// Restore backup
			exec.Command("mv", "/etc/resolv.conf.backup", "/etc/resolv.conf").Run()
			return
		}

		logger.LogMessage("INFO", "Updated DNS configuration to use Cloudflare DNS servers")

		// Test DNS resolution again
		cmd = exec.Command("timeout", "2", "getent", "hosts", "google.com")
		if err := cmd.Run(); err != nil {
			logger.LogMessage("ERROR", "DNS resolution still failing after configuration update")
		} else {
			logger.LogMessage("INFO", "DNS resolution working after configuration update")
		}
	}
}

// Function to verify the checksum of a file
func verifyChecksum(filePath, expectedChecksum string) bool {
	file, err := os.Open(filePath)
	if err != nil {
		logger.LogMessage("ERROR", fmt.Sprintf("Failed to open file for checksum verification: %s", err))
		return false
	}
	defer file.Close()

	hash := md5.New()
	if _, err := io.Copy(hash, file); err != nil {
		logger.LogMessage("ERROR", fmt.Sprintf("Failed to compute checksum: %s", err))
		return false
	}

	computedChecksum := hex.EncodeToString(hash.Sum(nil))
	return computedChecksum == expectedChecksum
}

// Function to check for updates and install them
func CheckForUpdates() {
	logger.LogMessage("INFO", "Checking for updates...")

	// Check and fix DNS
	checkAndFixDNS()

	metadataURL := config.Current.UpdaterService.MetadataURL
	username := config.Current.UpdaterService.Username
	password := config.Current.UpdaterService.Password

	// Create a new HTTP request
	req, err := http.NewRequest("GET", metadataURL, nil)
	if err != nil {
		logger.LogMessage("ERROR", fmt.Sprintf("Failed to create HTTP request: %s", err))
		return
	}

	req.SetBasicAuth(username, password)

	// Perform the HTTP request
	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		logger.LogMessage("ERROR", fmt.Sprintf("Failed to fetch update metadata: %s", err))
		return
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		logger.LogMessage("ERROR", fmt.Sprintf("Failed to fetch update metadata, status code: %d", resp.StatusCode))
		return
	}

	// Parse the metadata
	var metadata UpdateMetadata
	if err := json.NewDecoder(resp.Body).Decode(&metadata); err != nil {
		logger.LogMessage("ERROR", fmt.Sprintf("Failed to parse update metadata: %s", err))
		return
	}

	// Compare versions
	currentVersion := gatherer.GetCurrentVersion()
	if metadata.Version <= currentVersion {
		logger.LogMessage("INFO", "No new updates available.")
		return
	}

	logger.LogMessage("INFO", fmt.Sprintf("New version %s found, downloading update...", metadata.Version))

	// Create a new HTTP request for the update file
	updateReq, err := http.NewRequest("GET", metadata.URL, nil)
	if err != nil {
		logger.LogMessage("ERROR", fmt.Sprintf("Failed to create HTTP request for update: %s", err))
		return
	}

	// Set basic authentication for the update request
	updateReq.SetBasicAuth(username, password)

	// Perform the HTTP request to download the update
	updateResp, err := client.Do(updateReq)
	if err != nil {
		logger.LogMessage("ERROR", fmt.Sprintf("Failed to download update: %s", err))
		return
	}
	defer updateResp.Body.Close()

	if updateResp.StatusCode != http.StatusOK {
		logger.LogMessage("ERROR", fmt.Sprintf("Failed to download update, status code: %d", updateResp.StatusCode))
		return
	}

	// Create a temporary file to save the update
	tmpFile, err := os.CreateTemp("", "update-*.deb")
	if err != nil {
		logger.LogMessage("ERROR", fmt.Sprintf("Failed to create temp file for update: %s", err))
		return
	}
	defer os.Remove(tmpFile.Name())

	// Save the update to the temporary file
	_, err = io.Copy(tmpFile, updateResp.Body)
	if err != nil {
		logger.LogMessage("ERROR", fmt.Sprintf("Failed to save update: %s", err))
		return
	}

	// Verify the checksum
	if !verifyChecksum(tmpFile.Name(), metadata.Checksum) {
		logger.LogMessage("ERROR", "Checksum verification failed")
		return
	}

	// Install the update using dpkg
	cmd := exec.Command("sudo", "dpkg", "-i", tmpFile.Name())
	if err := cmd.Run(); err != nil {
		logger.LogMessage("ERROR", fmt.Sprintf("Failed to install update: %s", err))
		return
	}

	logger.LogMessage("INFO", "Update installed successfully. Restarting application...")
	os.Exit(0) // Exit to allow the system to restart the application
}
