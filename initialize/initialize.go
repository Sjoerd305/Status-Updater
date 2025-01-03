package initialize

import (
	"crypto/tls"
	"crypto/x509"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"status-updater/config"
	"status-updater/helpers"
	"status-updater/logger"
	"time"

	MQTT "github.com/eclipse/paho.mqtt.golang"
)

func LoadConfig() error {
	cwd, err := os.Getwd()
	if err != nil {
		return fmt.Errorf("failed to get current working directory: %v", err)
	}
	configFilePath := filepath.Join(cwd, "config.json")

	file, err := os.Open(configFilePath)
	if err != nil {
		return fmt.Errorf("configuration file not found at %s", configFilePath)
	}
	defer file.Close()

	decoder := json.NewDecoder(file)
	if err := decoder.Decode(&config.Current); err != nil {
		return fmt.Errorf("failed to decode configuration: %v", err)
	}

	// Required config validation
	if config.Current.UpdaterService.MetadataURL == "" {
		return fmt.Errorf("missing required configuration key: METADATA_URL")
	}
	if config.Current.UpdaterService.Username == "" {
		return fmt.Errorf("missing required configuration key: USERNAME")
	}
	if config.Current.UpdaterService.Password == "" {
		return fmt.Errorf("missing required configuration key: PASSWORD")
	}

	return nil
}

// MQTT client options initialization
func InitializeMQTTClientOptions() (*MQTT.ClientOptions, error) {
	// Config validation
	if config.Current.MQTT.Username == "" {
		return nil, fmt.Errorf("MQTT username not configured")
	}
	if config.Current.MQTT.Password == "" {
		return nil, fmt.Errorf("MQTT password not configured")
	}

	brokerAddress := helpers.ResolveBroker()
	logger.LogMessage("DEBUG", fmt.Sprintf("Resolved broker address: %s", brokerAddress))
	logger.LogMessage("DEBUG", fmt.Sprintf("Using username: %s", config.Current.MQTT.Username))

	opts := MQTT.NewClientOptions()
	brokerURL := fmt.Sprintf("ssl://%s:%d", brokerAddress, config.Current.MQTT.Port)
	opts.AddBroker(brokerURL)

	// Client ID from eth0 MAC
	eth0MAC, err := helpers.GetMACAddress("eth0")
	if err != nil {
		logger.LogMessage("ERROR", fmt.Sprintf("Failed to get MAC address for eth0: %s", err))
		eth0MAC = "unknown"
	}
	clientID := fmt.Sprintf("updater-%s", eth0MAC)
	opts.SetClientID(clientID)

	// Auth credentials
	opts.SetUsername(config.Current.MQTT.Username)
	opts.SetPassword(config.Current.MQTT.Password)

	// Connection stability params
	opts.SetConnectTimeout(30 * time.Second)
	opts.SetWriteTimeout(5 * time.Second)
	opts.SetKeepAlive(30 * time.Second)
	opts.SetPingTimeout(20 * time.Second)
	opts.SetMaxReconnectInterval(10 * time.Second)
	opts.SetAutoReconnect(true)
	opts.SetCleanSession(true)
	opts.SetOrderMatters(false)
	opts.SetResumeSubs(true)

	// TLS setup
	caCertPool, err := loadCACertificate()
	if err != nil {
		logger.LogMessage("ERROR", fmt.Sprintf("Failed to load CA certificate: %s", err))
		return nil, err
	}

	tlsConfig := &tls.Config{
		RootCAs:            caCertPool,
		MinVersion:         tls.VersionTLS12,
		InsecureSkipVerify: false,
	}
	opts.SetTLSConfig(tlsConfig)

	return opts, nil
}

// CA cert loader
func loadCACertificate() (*x509.CertPool, error) {
	caCertPool := x509.NewCertPool()

	caCert, err := os.ReadFile("cacert.pem")
	if err != nil {
		return nil, fmt.Errorf("failed to read CA certificate from file: %s", err)
	}

	if ok := caCertPool.AppendCertsFromPEM(caCert); !ok {
		return nil, fmt.Errorf("failed to append CA certificate from file")
	}

	logger.LogMessage("INFO", "Loaded CA certificate from file")
	return caCertPool, nil
}
