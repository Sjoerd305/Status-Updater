package mqtt

import (
	"context"
	"fmt"
	"status-updater-go/initialize"
	"status-updater-go/logger"
	"time"

	MQTT "github.com/eclipse/paho.mqtt.golang"
)

// Function to publish MQTT messages with retries
func PublishMQTTMessage(topic, message string) error {
	maxRetries := 3
	for attempt := 1; attempt <= maxRetries; attempt++ {
		logger.LogMessage("DEBUG", fmt.Sprintf("MQTT publish attempt %d/%d", attempt, maxRetries))

		opts, err := initialize.InitializeMQTTClientOptions()
		if err != nil {
			if attempt == maxRetries {
				return err
			}
			time.Sleep(time.Duration(attempt) * time.Second)
			continue
		}

		// Add connection status tracking
		connectionSuccess := make(chan bool, 1)
		connectionFailed := make(chan error, 1)

		opts.SetOnConnectHandler(func(client MQTT.Client) {
			logger.LogMessage("DEBUG", "Connected to MQTT broker")
			select {
			case connectionSuccess <- true:
			default:
			}
		})

		opts.SetConnectionLostHandler(func(client MQTT.Client, err error) {
			logger.LogMessage("WARN", fmt.Sprintf("Connection lost: %v", err))
			select {
			case connectionFailed <- err:
			default:
			}
		})

		// Create and connect client
		client := MQTT.NewClient(opts)

		if token := client.Connect(); token.Wait() && token.Error() != nil {
			logger.LogMessage("ERROR", fmt.Sprintf("Connection error: %v", token.Error()))
			client.Disconnect(250)
			if attempt == maxRetries {
				return token.Error()
			}
			time.Sleep(time.Duration(attempt) * time.Second)
			continue
		}

		// Wait for connection confirmation with timeout
		select {
		case <-connectionSuccess:
			// Continue with publish
		case err := <-connectionFailed:
			client.Disconnect(250)
			if attempt == maxRetries {
				return fmt.Errorf("connection failed: %v", err)
			}
			time.Sleep(time.Duration(attempt) * time.Second)
			continue
		case <-time.After(5 * time.Second):
			client.Disconnect(250)
			if attempt == maxRetries {
				return fmt.Errorf("connection timeout")
			}
			time.Sleep(time.Duration(attempt) * time.Second)
			continue
		}

		// Create a context with timeout for publish operation
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		publishComplete := make(chan error, 1)

		go func() {
			token := client.Publish(topic, 1, false, message)
			token.Wait()
			publishComplete <- token.Error()
		}()

		// Wait for publish completion or timeout
		select {
		case err := <-publishComplete:
			if err != nil {
				logger.LogMessage("ERROR", fmt.Sprintf("Publish error: %v", err))
				client.Disconnect(250)
				if attempt == maxRetries {
					return err
				}
				time.Sleep(time.Duration(attempt) * time.Second)
				continue
			}
			logger.LogMessage("INFO", fmt.Sprintf("Successfully published message to %s", topic))
			// Small delay before disconnect to ensure message is fully sent
			time.Sleep(100 * time.Millisecond)
			client.Disconnect(250)
			return nil
		case <-ctx.Done():
			client.Disconnect(250)
			if attempt == maxRetries {
				return fmt.Errorf("publish timeout")
			}
			time.Sleep(time.Duration(attempt) * time.Second)
			continue
		}
	}

	return fmt.Errorf("failed to publish after %d attempts", maxRetries)
}
