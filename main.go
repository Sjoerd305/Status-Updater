package main

import (
	"context"
	"encoding/json"
	"fmt"
	"math/rand"
	"status-updater/config"
	"status-updater/gatherer"
	"status-updater/helpers"
	"status-updater/initialize"
	"status-updater/logger"
	"status-updater/mqtt"
	"status-updater/system"
	"status-updater/updater"
	"strconv"
	"sync"
	"time"
)

func main() {
	defer system.RecoverFromPanic()
	if err := initialize.LoadConfig(); err != nil {
		logger.LogMessage("ERROR", fmt.Sprintf("Failed to load configuration: %v", err))
	}

	// Check if LOG_FILE is set
	if config.Current.Log.File == "" {
		logger.LogMessage("ERROR", "LOG_FILE is not set in the configuration")
	} else {
		logger.LogMessage("INFO", fmt.Sprintf("LOG_FILE is set to: %s", config.Current.Log.File))
	}

	// Log start message
	logger.LogMessage("INFO", "Status Updater started")

	// Get the device type
	deviceType, err := gatherer.GetDeviceType()
	if err != nil {
		logger.LogMessage("ERROR", fmt.Sprintf("Failed to determine device type: %v", err))
	}
	logger.LogMessage("INFO", fmt.Sprintf("Device type: %s", deviceType))

	// Parse the sleep interval from the config
	sleepIntervalStr := fmt.Sprintf("%d", config.Current.SleepInterval)
	if sleepIntervalStr == "" {
		logger.LogMessage("ERROR", "SLEEP_INTERVAL is not set in the configuration")
		sleepIntervalStr = "300" // Default to 300 seconds if not set
	}
	sleepInterval, err := strconv.Atoi(sleepIntervalStr)
	if err != nil {
		logger.LogMessage("ERROR", fmt.Sprintf("Invalid SLEEP_INTERVAL in config: %s", err))
		sleepInterval = 300 // Default to 300 seconds if parsing fails
	}

	// Create a context that we can cancel
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	var wg sync.WaitGroup

	wg.Add(1)
	go func() {
		defer wg.Done()
		system.MonitorNetworkChanges(ctx)
	}()

	// Function to send status update with retry logic
	sendStatusUpdate := func() {
		maxRetries := 3
		retryDelay := time.Second * 180

		for attempt := 1; attempt <= maxRetries; attempt++ {
			logger.LogMessage("DEBUG", fmt.Sprintf("Starting status update (attempt %d/%d)...", attempt, maxRetries))

			// Check internet connectivity
			if !helpers.IsInternetAvailable() {
				logger.LogMessage("WARN", fmt.Sprintf("No internet connection (attempt %d/%d), waiting %v before retry",
					attempt, maxRetries, retryDelay))
				if attempt < maxRetries {
					time.Sleep(retryDelay)
					continue
				}
				return
			}

			// Wrap the entire status update in a recovery block
			func() {
				defer func() {
					if r := recover(); r != nil {
						logger.LogMessage("ERROR", fmt.Sprintf("Recovered from panic in status update: %v", r))
					}
				}()

				ipAddress := gatherer.GetIPAddresses()
				macAddress := gatherer.GetMACAddresses()
				modemDetails := gatherer.GetModemDetails()
				temperature := gatherer.GetTemperature()
				switchName, switchIP, switchPort, switchMacAddress, switchPortVlan, switchSysDescription, switchPortDescription := gatherer.GetLLDPDetails()

				// Check if WLAN is enabled in the configuration
				var ssid, apMAC string
				if helpers.HasActiveWLANInterface() {
					ssid = helpers.GetSSID()
					apMAC = gatherer.GetAccessPointMAC() // Get the MAC address of the access point
					logger.LogMessage("DEBUG", fmt.Sprintf("Found WLAN interface with SSID: %s and AP MAC: %s", ssid, apMAC))
				} else {
					ssid = "N/A"
					apMAC = "N/A"
					logger.LogMessage("DEBUG", "No active WLAN interface found")
				}

				// Get the MAC address of eth0
				eth0MAC, err := helpers.GetMACAddress("eth0")
				if err != nil {
					logger.LogMessage("ERROR", fmt.Sprintf("Failed to get MAC address for eth0: %s", err))
					eth0MAC = "unknown"
				}

				// Read updater version
				updaterVersion := helpers.GetUpdaterVersion()

				// Read Helpcom configuration
				helpcomConfig, err := gatherer.ReadHelpcomConfig()
				if err != nil {
					logger.LogMessage("ERROR", fmt.Sprintf("Failed to read Helpcom configuration: %s", err))
				}

				// Get the uptime of the device
				uptime := gatherer.GetUptime()

				// Get the Linux version
				linuxVersion := gatherer.GetLinuxVersion()

				// Create a JSON message
				message := map[string]interface{}{
					"status":                  "Online",
					"services":                gatherer.GetServiceStatus(),
					"date":                    time.Now().UTC().Format(time.RFC3339),
					"deviceID":                eth0MAC,
					"device_type":             deviceType,
					"ip_addresses":            json.RawMessage(ipAddress),
					"mac_addresses":           json.RawMessage(macAddress),
					"modem":                   json.RawMessage(modemDetails),
					"temp":                    temperature,
					"switch_name":             switchName,
					"switch_ip":               switchIP,
					"switch_port":             switchPort,
					"switch_mac_address":      switchMacAddress,
					"switch_port_vlan":        switchPortVlan,
					"switch_sys_description":  switchSysDescription,
					"switch_port_description": switchPortDescription,
					"wifi_ssid":               ssid,
					"wifi_ap_mac":             apMAC,
					"updater_version":         updaterVersion,
					"helpcom_servers":         helpcomConfig["HelpcomServers"],
					"helpcom_lifespan":        helpcomConfig["HelpcomLifespan"],
					"helpcom_rf":              helpcomConfig["HelpcomRF"],
					"uptime":                  uptime,
					"os_version":              linuxVersion,
				}
				messageJSON, err := json.Marshal(message)
				if err != nil {
					logger.LogMessage("ERROR", fmt.Sprintf("Failed to marshal JSON: %s", err))
					return
				}

				// Construct the topic
				topic := fmt.Sprintf("%s/status", eth0MAC)
				logger.LogMessage("INFO", fmt.Sprintf("Sending message to topic: %s", topic))
				err = mqtt.PublishMQTTMessage(topic, string(messageJSON))
				if err != nil {
					logger.LogMessage("ERROR", fmt.Sprintf("Failed to publish message (attempt %d/%d): %s",
						attempt, maxRetries, err))
					if attempt < maxRetries {
						time.Sleep(retryDelay)
						return
					}
				} else {
					logger.LogMessage("DEBUG", "Status update completed successfully.")
					return
				}
			}()

			// Only retry if there was an error
			if err != nil {
				logger.LogMessage("ERROR", fmt.Sprintf("Retrying due to error: %v", err))
				if attempt < maxRetries {
					time.Sleep(retryDelay)
				}
			} else {
				break
			}
		}
	}

	// Run the main loop in a separate goroutine
	go func() {
		// Send initial status update immediately
		sendStatusUpdate()

		// Calculate a random delay within the next 4 hours
		randomDelay := time.Duration(rand.Intn(4*60*60)) * time.Second
		logger.LogMessage("INFO", fmt.Sprintf("Next update check in %v at %s", randomDelay, time.Now().Add(randomDelay).Format(time.RFC3339)))

		// Wait for the random delay before starting the ticker
		select {
		case <-time.After(randomDelay):
		case <-ctx.Done():
			return
		}

		// Then start the regular interval updates
		ticker := time.NewTicker(time.Duration(sleepInterval) * time.Second)
		defer ticker.Stop()

		for {
			select {
			case <-ticker.C:
				sendStatusUpdate()
			case <-ctx.Done():
				logger.LogMessage("INFO", "Context cancelled, stopping the main loop")
				return
			}
		}
	}()

	// Check for updates on startup
	updater.CheckForUpdates()

	// Run the update check in a separate goroutine
	go func() {
		for {
			// Calculate a random delay within the next 24 hours
			randomDelay := time.Duration(rand.Intn(24*60*60)) * time.Second
			logger.LogMessage("INFO", fmt.Sprintf("Next update check in %v at %s", randomDelay, time.Now().Add(randomDelay).Format(time.RFC3339)))

			// Wait for the random delay
			select {
			case <-time.After(randomDelay):
				updater.CheckForUpdates()
			case <-ctx.Done():
				return
			}
		}
	}()

	// Wait for termination signal
	system.HandleShutdown(cancel, &wg)

	wg.Wait()
	logger.LogMessage("INFO", "All goroutines have completed.")
}
