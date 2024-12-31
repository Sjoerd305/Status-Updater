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
	"path/filepath"
	"status-updater/config"
	"status-updater/helpers"
	"status-updater/logger"
)

func checkAndFixDNS() {
	// Check wwan0 interface status
	cmd := exec.Command("ip", "link", "show", "wwan0")
	if err := cmd.Run(); err != nil {
		logger.LogMessage("DEBUG", "wwan0 interface not found, skipping DNS check")
		return
	}

	// DNS resolution test
	cmd = exec.Command("timeout", "2", "getent", "hosts", "google.com")
	if err := cmd.Run(); err != nil {
		logger.LogMessage("WARN", "DNS resolution failed, attempting to fix DNS configuration")

		// Backup resolv.conf
		if err := exec.Command("cp", "/etc/resolv.conf", "/etc/resolv.conf.backup").Run(); err != nil {
			logger.LogMessage("ERROR", fmt.Sprintf("Failed to backup resolv.conf: %v", err))
			return
		}

		// Set Cloudflare DNS
		dnsConfig := []byte("nameserver 1.1.1.1\nnameserver 1.0.0.1\n")
		if err := os.WriteFile("/etc/resolv.conf", dnsConfig, 0644); err != nil {
			logger.LogMessage("ERROR", fmt.Sprintf("Failed to update resolv.conf: %v", err))
			// Restore from backup
			exec.Command("mv", "/etc/resolv.conf.backup", "/etc/resolv.conf").Run()
			return
		}

		logger.LogMessage("INFO", "Updated DNS configuration to use Cloudflare DNS servers")

		// Verify DNS fix
		cmd = exec.Command("timeout", "2", "getent", "hosts", "google.com")
		if err := cmd.Run(); err != nil {
			logger.LogMessage("ERROR", "DNS resolution still failing after configuration update")
		} else {
			logger.LogMessage("INFO", "DNS resolution working after configuration update")
		}
	}
}

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

func CheckForUpdates() {
	logger.LogMessage("INFO", "Checking for updates...")

	checkAndFixDNS()

	if helpers.IsBuildroot() {
		UpdateBuildroot()
		return
	}

	// Debian update flow
	metadataURL := config.Current.UpdaterService.MetadataURL
	username := config.Current.UpdaterService.Username
	password := config.Current.UpdaterService.Password

	req, err := http.NewRequest("GET", metadataURL, nil)
	if err != nil {
		logger.LogMessage("ERROR", fmt.Sprintf("Failed to create HTTP request: %s", err))
		return
	}

	req.SetBasicAuth(username, password)

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

	var metadata struct {
		Version        string `json:"version"`
		DebianURL      string `json:"debian_url"`
		DebianChecksum string `json:"debian_checksum"`
		ReleaseNotes   string `json:"release_notes"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&metadata); err != nil {
		logger.LogMessage("ERROR", fmt.Sprintf("Failed to parse update metadata: %s", err))
		return
	}

	if metadata.Version == "" || metadata.DebianURL == "" || metadata.DebianChecksum == "" {
		logger.LogMessage("ERROR", "Invalid update metadata received")
		return
	}

	currentVersion := helpers.GetUpdaterVersion()
	if metadata.Version <= currentVersion {
		logger.LogMessage("INFO", "No new updates available.")
		return
	}

	logger.LogMessage("INFO", fmt.Sprintf("New version %s found, downloading update...", metadata.Version))

	updateReq, err := http.NewRequest("GET", metadata.DebianURL, nil)
	if err != nil {
		logger.LogMessage("ERROR", fmt.Sprintf("Failed to create HTTP request for update: %s", err))
		return
	}

	updateReq.SetBasicAuth(username, password)

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

	tmpFile, err := os.CreateTemp("", "update-*.deb")
	if err != nil {
		logger.LogMessage("ERROR", fmt.Sprintf("Failed to create temp file for update: %s", err))
		return
	}
	defer os.Remove(tmpFile.Name())

	_, err = io.Copy(tmpFile, updateResp.Body)
	if err != nil {
		logger.LogMessage("ERROR", fmt.Sprintf("Failed to save update: %s", err))
		return
	}

	if !verifyChecksum(tmpFile.Name(), metadata.DebianChecksum) {
		logger.LogMessage("ERROR", "Checksum verification failed")
		return
	}

	cmd := exec.Command("sudo", "dpkg", "-i", tmpFile.Name())
	if err := cmd.Run(); err != nil {
		logger.LogMessage("ERROR", fmt.Sprintf("Failed to install update: %s", err))
		return
	}

	logger.LogMessage("INFO", "Update installed successfully. Restarting application...")
	os.Exit(0) // Force restart via service manager
}

func UpdateBuildroot() {

	metadataURL := config.Current.UpdaterService.MetadataURL
	username := config.Current.UpdaterService.Username
	password := config.Current.UpdaterService.Password

	req, err := http.NewRequest("GET", metadataURL, nil)
	if err != nil {
		logger.LogMessage("ERROR", fmt.Sprintf("Failed to create HTTP request: %s", err))
		return
	}

	req.SetBasicAuth(username, password)

	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		logger.LogMessage("ERROR", fmt.Sprintf("Failed to fetch update metadata: %s", err))
		return
	}
	defer resp.Body.Close()

	var metadata struct {
		Version           string `json:"version"`
		BuildrootURL      string `json:"buildroot_url"`
		BuildrootChecksum string `json:"buildroot_checksum"`
		ReleaseNotes      string `json:"release_notes"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&metadata); err != nil {
		logger.LogMessage("ERROR", fmt.Sprintf("Failed to parse update metadata: %s", err))
		return
	}

	if metadata.Version == "" || metadata.BuildrootURL == "" || metadata.BuildrootChecksum == "" {
		logger.LogMessage("ERROR", "Invalid update metadata received")
		return
	}

	logger.LogMessage("INFO", fmt.Sprintf("New version %s found, downloading update...", metadata.Version))

	updateReq, err := http.NewRequest("GET", metadata.BuildrootURL, nil)
	if err != nil {
		logger.LogMessage("ERROR", fmt.Sprintf("Failed to create HTTP request for update: %s", err))
		return
	}

	updateReq.SetBasicAuth(username, password)

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

	tmpDir, err := os.MkdirTemp("", "update-*")
	if err != nil {
		logger.LogMessage("ERROR", fmt.Sprintf("Failed to create temp directory for update: %s", err))
		return
	}
	defer os.RemoveAll(tmpDir)

	tmpFile := filepath.Join(tmpDir, "update.tar.xz")
	f, err := os.Create(tmpFile)
	if err != nil {
		logger.LogMessage("ERROR", fmt.Sprintf("Failed to create temp file for update: %s", err))
		return
	}

	_, err = io.Copy(f, updateResp.Body)
	if err != nil {
		logger.LogMessage("ERROR", fmt.Sprintf("Failed to save update: %s", err))
		return
	}
	f.Close()

	if !verifyChecksum(tmpFile, metadata.BuildrootChecksum) {
		logger.LogMessage("ERROR", "Checksum verification failed")
		return
	}

	// Extract the update to temp directory
	cmd := exec.Command("tar", "-xJf", tmpFile, "-C", tmpDir)
	if err := cmd.Run(); err != nil {
		logger.LogMessage("ERROR", fmt.Sprintf("Failed to extract update: %s", err))
		return
	}

	// Run deploy script
	deployCmd := exec.Command("./deploy.sh")
	deployCmd.Dir = tmpDir
	if err := deployCmd.Run(); err != nil {
		logger.LogMessage("ERROR", fmt.Sprintf("Failed to run deploy script: %s", err))
		return
	}

	logger.LogMessage("INFO", "Update installed successfully. Restarting application...")
	os.Exit(0) // Force restart via service manager
}
