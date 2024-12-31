package main

import (
	"context"
	"encoding/json"
	"fmt"
	"math/rand"
	"os"
	"reflect"
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

var (
	messageBuffer map[string]interface{}
	bufferMutex   sync.RWMutex
)

func main() {
	defer system.RecoverFromPanic()
	if err := initialize.LoadConfig(); err != nil {
		logger.LogMessage("ERROR", fmt.Sprintf("Failed to load configuration: %v", err))
	}

	// LOG_FILE validation
	if config.Current.Log.File == "" {
		logger.LogMessage("ERROR", "LOG_FILE is not set in the configuration")
	} else {
		logger.LogMessage("INFO", fmt.Sprintf("LOG_FILE is set to: %s", config.Current.Log.File))
	}

	logger.LogMessage("INFO", "Status Updater started")

	deviceType, err := gatherer.GetDeviceType()
	if err != nil {
		logger.LogMessage("ERROR", fmt.Sprintf("Failed to determine device type: %v", err))
	}
	logger.LogMessage("INFO", fmt.Sprintf("Device type: %s", deviceType))

	// Default sleep interval: 300s
	sleepIntervalStr := fmt.Sprintf("%d", config.Current.SleepInterval)

	logger.LogMessage("INFO", fmt.Sprintf("Sleep interval: %s", sleepIntervalStr))
	if sleepIntervalStr == "" {
		logger.LogMessage("ERROR", "Sleep interval is not set in the configuration")
		sleepIntervalStr = "300"
	}
	sleepInterval, err := strconv.Atoi(sleepIntervalStr)
	if err != nil {
		logger.LogMessage("ERROR", fmt.Sprintf("Invalid Sleep interval in config: %s", err))
		sleepInterval = 300
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	var wg sync.WaitGroup

	wg.Add(1)
	go func() {
		defer wg.Done()
		system.MonitorNetworkChanges(ctx)
	}()

	// Initialize message buffer
	messageBuffer = make(map[string]interface{})

	// Status update with retries
	sendStatusUpdate := func() {
		maxRetries := 3
		retryDelay := time.Second * 180

		for attempt := 1; attempt <= maxRetries; attempt++ {
			logger.LogMessage("DEBUG", fmt.Sprintf("Starting status update (attempt %d/%d)...", attempt, maxRetries))

			if !helpers.IsInternetAvailable() {
				logger.LogMessage("WARN", fmt.Sprintf("No internet connection (attempt %d/%d), waiting %v before retry",
					attempt, maxRetries, retryDelay))
				if attempt < maxRetries {
					time.Sleep(retryDelay)
					continue
				}
				return
			}

			// Panic recovery wrapper
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

				// WLAN interface check
				var ssid, apMAC string
				if helpers.HasActiveWLANInterface() {
					ssid = helpers.GetSSID()
					apMAC = gatherer.GetAccessPointMAC()
					logger.LogMessage("DEBUG", fmt.Sprintf("Found WLAN interface with SSID: %s and AP MAC: %s", ssid, apMAC))
				} else {
					ssid = "N/A"
					apMAC = "N/A"
					logger.LogMessage("DEBUG", "No active WLAN interface found")
				}

				eth0MAC, err := helpers.GetMACAddress("eth0")
				if err != nil {
					logger.LogMessage("ERROR", fmt.Sprintf("Failed to get MAC address for eth0: %s", err))
					eth0MAC = "unknown"
				}

				updaterVersion := helpers.GetUpdaterVersion()

				helpcomConfig, err := gatherer.ReadHelpcomConfig()
				if err != nil {
					logger.LogMessage("ERROR", fmt.Sprintf("Failed to read Helpcom configuration: %s", err))
				}

				uptime := gatherer.GetUptime()
				linuxVersion := gatherer.GetLinuxVersion()

				// Status payload
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

				// Compare with buffer and only send changed fields
				bufferMutex.RLock()
				isFirstRun := len(messageBuffer) == 0
				changedFields := make(map[string]interface{})

				if isFirstRun {
					changedFields = message
				} else {
					// Always include status and deviceID fields
					changedFields["status"] = "Online"
					changedFields["deviceID"] = eth0MAC

					// Check other fields for changes
					for key, value := range message {
						if key != "status" && key != "deviceID" && !reflect.DeepEqual(messageBuffer[key], value) {
							changedFields[key] = value
						}
					}
				}
				bufferMutex.RUnlock()

				// If there are changes or it's the first run, send the update
				if len(changedFields) > 0 {
					messageJSON, err := json.Marshal(changedFields)
					if err != nil {
						logger.LogMessage("ERROR", fmt.Sprintf("Failed to marshal JSON: %s", err))
						return
					}

					topic := fmt.Sprintf("%s/status", eth0MAC)
					logger.LogMessage("INFO", fmt.Sprintf("Sending message to topic: %s with %d changed fields", topic, len(changedFields)))
					err = mqtt.PublishMQTTMessage(topic, string(messageJSON))
					if err != nil {
						logger.LogMessage("ERROR", fmt.Sprintf("Failed to publish message (attempt %d/%d): %s",
							attempt, maxRetries, err))
						if attempt < maxRetries {
							time.Sleep(retryDelay)
							return
						}
					} else {
						// Update buffer with new values
						bufferMutex.Lock()
						for k, v := range changedFields {
							messageBuffer[k] = v
						}
						bufferMutex.Unlock()

						logger.LogMessage("DEBUG", fmt.Sprintf("Status update completed successfully with %d changes.", len(changedFields)))
						return
					}
				} else {
					logger.LogMessage("DEBUG", "No changes detected, skipping status update.")
					return
				}
			}()

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

	// Main update loop
	go func() {
		sendStatusUpdate()

		// Random initial delay (4h max) only on first run
		if _, err := os.Stat("/var/run/status-updater.initialized"); os.IsNotExist(err) {
			randomDelay := time.Duration(rand.Intn(4*60*60)) * time.Second
			logger.LogMessage("INFO", fmt.Sprintf("Initial startup delay of %v until %s", randomDelay, time.Now().Add(randomDelay).Format(time.RFC3339)))

			select {
			case <-time.After(randomDelay):
				// Create initialization marker file
				if err := os.WriteFile("/var/run/status-updater.initialized", []byte{}, 0644); err != nil {
					logger.LogMessage("ERROR", fmt.Sprintf("Failed to create initialization marker: %v", err))
				}
			case <-ctx.Done():
				return
			}
		}

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

	updater.CheckForUpdates()

	// Update checker loop
	go func() {
		for {
			// Random check interval (24h max)
			randomDelay := time.Duration(rand.Intn(24*60*60)) * time.Second
			logger.LogMessage("INFO", fmt.Sprintf("Next update check in %v at %s", randomDelay, time.Now().Add(randomDelay).Format(time.RFC3339)))

			select {
			case <-time.After(randomDelay):
				updater.CheckForUpdates()
			case <-ctx.Done():
				return
			}
		}
	}()

	system.HandleShutdown(cancel, &wg)

	wg.Wait()
	logger.LogMessage("INFO", "All goroutines have completed.")
}
