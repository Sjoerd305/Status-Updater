package main

import (
	"bufio"
	"bytes"
	"encoding/json"
	"fmt"
	"log"
	"math/rand"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"golang.org/x/crypto/ssh"
)

func main() {
	config, err := os.ReadFile("config.json")
	if err != nil {
		fmt.Printf("Failed to read config.json: %v\n", err)
		return
	}

	var configMap map[string]string
	if err := json.Unmarshal(config, &configMap); err != nil {
		fmt.Printf("Failed to unmarshal config.json: %v\n", err)
		return
	}

	logFile, err := os.OpenFile("installer.log", os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		fmt.Printf("Failed to open log file: %v\n", err)
		return
	}
	defer logFile.Close()
	log.SetOutput(logFile)

	fmt.Println("Select device type:")
	fmt.Println("1. HC9XX device")
	fmt.Println("2. SOS device")

	var choice string
	fmt.Print("Enter your choice (1 or 2): ")
	fmt.Scanln(&choice)

	var usernames []string
	var credentials map[string]string

	switch choice {
	case "1":
		usernames = []string{configMap["username1"]}
		credentials = map[string]string{
			configMap["username1"]: configMap["password1"],
		}
	case "2":
		usernames = []string{configMap["username2"]}
		credentials = map[string]string{
			configMap["username2"]: configMap["password2"],
		}
	default:
		logAndPrint("Invalid choice. Exiting.")
		return
	}

	ips, err := readIPsFromFile("iplist")
	if err != nil {
		logAndPrint(fmt.Sprintf("Failed to read IP list: %v\n", err))
		return
	}

	port := "22"

	debFiles, err := filepath.Glob("*.deb")
	if err != nil || len(debFiles) == 0 {
		logAndPrint("No .deb files found in the current directory.")
		return
	}

	fmt.Println("Select the .deb file to install:")
	for i, file := range debFiles {
		fmt.Printf("%d. %s\n", i+1, file)
	}

	var debChoice int
	fmt.Print("Enter your choice: ")
	fmt.Scanln(&debChoice)

	if debChoice < 1 || debChoice > len(debFiles) {
		logAndPrint("Invalid choice. Exiting.")
		return
	}

	debFile := debFiles[debChoice-1]

	debData, err := os.ReadFile(debFile)
	if err != nil {
		logAndPrint(fmt.Sprintf("Failed to read .deb file: %v\n", err))
		return
	}

	fmt.Print("Do you want to install lldpd on all devices? (y/n): ")
	var lldpdChoice string
	fmt.Scanln(&lldpdChoice)
	installLldpd := strings.ToLower(lldpdChoice) == "y"

	var wg sync.WaitGroup
	sem := make(chan struct{}, 10) // Max 10 concurrent connections
	var failedInstalls []string
	var mu sync.Mutex

	for _, host := range ips {
		wg.Add(1)
		go func(host string) {
			defer wg.Done()
			sem <- struct{}{}
			defer func() { <-sem }()

			logAndPrint(fmt.Sprintf("Processing host: %s\n", host))

			var client *ssh.Client
			var err error
			var successfulUser string

			for _, user := range usernames {
				client, err = connectSSH(host, user, credentials[user], port)
				if err == nil {
					successfulUser = user
					break
				}
				logAndPrint(fmt.Sprintf("Failed to connect to %s with user %s: %v\n", host, user, err))
			}

			if err != nil {
				logAndPrint(fmt.Sprintf("Failed to connect to %s with any user\n", host))
				mu.Lock()
				failedInstalls = append(failedInstalls, host)
				mu.Unlock()
				return
			}
			defer client.Close()

			isBuildroot := checkBuildroot(client)
			if isBuildroot {
				err = installBuildroot(client)
			} else {
				err = installDeb(client, debData, debFile, credentials[successfulUser], installLldpd)
			}

			if err != nil {
				logAndPrint(fmt.Sprintf("Failed to install on %s: %v\n", host, err))
				mu.Lock()
				failedInstalls = append(failedInstalls, host)
				mu.Unlock()
			} else {
				logAndPrint(fmt.Sprintf("Successfully installed on %s\n", host))
			}
		}(host)
	}

	wg.Wait()

	if len(failedInstalls) > 0 {
		logAndPrint("Failed installs on the following hosts:")
		for _, host := range failedInstalls {
			logAndPrint(host)
		}
	}

	logAndPrint(fmt.Sprintf("Total hosts: %d", len(ips)))
	logAndPrint(fmt.Sprintf("Successful installs: %d", len(ips)-len(failedInstalls)))
	logAndPrint(fmt.Sprintf("Failed installs: %d", len(failedInstalls)))
}

func readIPsFromFile(filename string) ([]string, error) {
	file, err := os.Open(filename)
	if err != nil {
		return nil, err
	}
	defer file.Close()

	var ips []string
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line != "" {
			ips = append(ips, line)
		}
	}

	if err := scanner.Err(); err != nil {
		return nil, err
	}

	return ips, nil
}

func transferFile(client *ssh.Client, data []byte, remotePath string) error {
	session, err := client.NewSession()
	if err != nil {
		return fmt.Errorf("failed to create session: %v", err)
	}
	defer session.Close()

	var stderr bytes.Buffer
	session.Stderr = &stderr

	go func() {
		w, _ := session.StdinPipe()
		defer w.Close()

		fmt.Fprintf(w, "C0644 %d %s\n", len(data), filepath.Base(remotePath))
		w.Write(data)
		fmt.Fprint(w, "\x00")
	}()

	scpCmd := fmt.Sprintf("/usr/bin/scp -o StrictHostKeyChecking=no -o UserKnownHostsFile=/dev/null -t %s", remotePath)
	logAndPrint(fmt.Sprintf("Running SCP command: %s", scpCmd))
	if err := session.Run(scpCmd); err != nil {
		return fmt.Errorf("scp command failed: %v, stderr: %s", err, stderr.String())
	}

	return nil
}

func logAndPrint(message string) {
	log.Print(message)
	fmt.Println(message)
}

func connectSSH(host, user, password, port string) (*ssh.Client, error) {
	const maxRetries = 3
	var client *ssh.Client
	var err error

	for i := 0; i < maxRetries; i++ {
		config := &ssh.ClientConfig{
			User: user,
			Auth: []ssh.AuthMethod{
				ssh.Password(password),
			},
			HostKeyCallback: ssh.InsecureIgnoreHostKey(),
			Timeout:         10 * time.Second,
		}
		client, err = ssh.Dial("tcp", host+":"+port, config)
		if err == nil {
			return client, nil
		}
		logAndPrint(fmt.Sprintf("SSH connection to %s@%s:%s failed (attempt %d/%d): %v", user, host, port, i+1, maxRetries, err))
		time.Sleep(2 * time.Second)
	}

	return nil, fmt.Errorf("SSH connection to %s@%s:%s failed after %d attempts: %v", user, host, port, maxRetries, err)
}

func checkBuildroot(client *ssh.Client) bool {
	session, err := client.NewSession()
	if err != nil {
		return false
	}
	defer session.Close()

	var stdout bytes.Buffer
	session.Stdout = &stdout
	err = session.Run("cat /etc/os-release")
	if err != nil {
		return false
	}

	return strings.Contains(stdout.String(), "Buildroot")
}

func installBuildroot(client *ssh.Client) error {
	files := map[string]string{
		"status-updater": "/opt/status-updater/status-updater",
		"cacert.pem":     "/opt/status-updater/cacert.pem",
		"config":         "/opt/status-updater/config",
	}

	for localFile := range files {
		if _, err := os.Stat(localFile); os.IsNotExist(err) {
			return fmt.Errorf("local file %s does not exist", localFile)
		}
	}

	for _, remotePath := range files {
		dir := filepath.Dir(remotePath)
		session, err := client.NewSession()
		if err != nil {
			return fmt.Errorf("failed to create session: %v", err)
		}
		err = session.Run(fmt.Sprintf("mkdir -p %s", dir))
		session.Close()
		if err != nil {
			return fmt.Errorf("failed to create directory %s: %v", dir, err)
		}
	}

	for localFile, remoteFile := range files {
		data, err := os.ReadFile(localFile)
		if err != nil {
			return fmt.Errorf("failed to read file %s: %v", localFile, err)
		}
		err = transferFile(client, data, remoteFile)
		if err != nil {
			return fmt.Errorf("failed to transfer file %s: %v", localFile, err)
		}
	}

	rand.Seed(time.Now().UnixNano())
	randomDelay := rand.Intn(600)

	initScript := fmt.Sprintf(`#!/bin/sh
### BEGIN INIT INFO
# Provides:          status-updater
# Required-Start:    $remote_fs $syslog
# Required-Stop:     $remote_fs $syslog
# Default-Start:     2 3 4 5
# Default-Stop:      0 1 6
# Short-Description: Start daemon at boot time
# Description:       Enable service provided by daemon.
### END INIT INFO

DAEMON_PATH="/opt/status-updater"
DAEMON="$DAEMON_PATH/status-updater"
DAEMON_NAME="status-updater"
PIDFILE="/var/run/$DAEMON_NAME.pid"
LOGFILE="/var/log/$DAEMON_NAME.log"

. /lib/lsb/init-functions

do_start() {
    log_daemon_msg "Starting $DAEMON_NAME"
    sleep %d
    start-stop-daemon --start --background --make-pidfile --pidfile $PIDFILE --chdir $DAEMON_PATH --exec $DAEMON -- >> $LOGFILE 2>&1
    log_end_msg $?
}

do_stop() {
    log_daemon_msg "Stopping $DAEMON_NAME"
    start-stop-daemon --stop --pidfile $PIDFILE --retry 10
    log_end_msg $?
}

case "$1" in
  start)
    do_start
    ;;
  stop)
    do_stop
    ;;
  restart)
    do_stop
    do_start
    ;;
  status)
    status_of_proc -p $PIDFILE $DAEMON $DAEMON_NAME && exit 0 || exit $?
    ;;
  *)
    echo "Usage: /etc/init.d/$DAEMON_NAME {start|stop|restart|status}"
    exit 1
    ;;
esac
exit 0`, randomDelay)

	err := transferFile(client, []byte(initScript), "/etc/init.d/status-updater")
	if err != nil {
		return fmt.Errorf("failed to create init script: %v", err)
	}

	session, err := client.NewSession()
	if err != nil {
		return fmt.Errorf("failed to create session: %v", err)
	}
	defer session.Close()

	err = session.Run("chmod +x /etc/init.d/status-updater")
	if err != nil {
		return fmt.Errorf("failed to make init script executable: %v", err)
	}

	err = session.Run("update-rc.d status-updater defaults")
	if err != nil {
		return fmt.Errorf("failed to enable service: %v", err)
	}

	session, err = client.NewSession()
	if err != nil {
		return fmt.Errorf("failed to create session for service start: %v", err)
	}
	defer session.Close()

	err = session.Run("/etc/init.d/status-updater start")
	if err != nil {
		return fmt.Errorf("failed to start service: %v", err)
	}

	session, err = client.NewSession()
	if err != nil {
		return fmt.Errorf("failed to create session for status check: %v", err)
	}
	defer session.Close()

	var stdout bytes.Buffer
	session.Stdout = &stdout
	err = session.Run("ps aux | grep status-updater | grep -v grep")
	if err != nil {
		return fmt.Errorf("service verification failed - status-updater might not be running: %v", err)
	}

	return nil
}

func installDeb(client *ssh.Client, debData []byte, debFile string, password string, installLldpd bool) error {
	if installLldpd {
		zipFile := "lldpd-packages.zip"
		zipData, err := os.ReadFile(zipFile)
		if err != nil {
			return fmt.Errorf("failed to read zip file: %v", err)
		}

		remoteZipFile := "/tmp/" + filepath.Base(zipFile)
		err = transferFile(client, zipData, remoteZipFile)
		if err != nil {
			return fmt.Errorf("failed to transfer zip file: %v", err)
		}

		session, err := client.NewSession()
		if err != nil {
			return fmt.Errorf("failed to create session for zip handling: %v", err)
		}
		defer session.Close()

		var stderr bytes.Buffer
		session.Stderr = &stderr

		cmd := fmt.Sprintf(`
			unzip -o %s -d /tmp/lldpd-packages && \
			echo %s | sudo -S dpkg -i /tmp/lldpd-packages/*.deb && \
			rm -rf /tmp/lldpd-packages %s
		`, remoteZipFile, password, remoteZipFile)

		err = session.Run(cmd)
		if err != nil {
			return fmt.Errorf("failed to install lldpd from zip: %v, stderr: %s", err, stderr.String())
		}
	}

	remoteFile := "/tmp/" + filepath.Base(debFile)
	err := transferFile(client, debData, remoteFile)
	if err != nil {
		return fmt.Errorf("failed to transfer file: %v", err)
	}

	session, err := client.NewSession()
	if err != nil {
		return fmt.Errorf("failed to create session: %v", err)
	}
	defer session.Close()

	var stderr bytes.Buffer
	session.Stderr = &stderr

	cmd := fmt.Sprintf("echo %s | sudo -S dpkg -i %s", password, remoteFile)
	err = session.Run(cmd)
	if err != nil {
		return fmt.Errorf("failed to install .deb file: %v, stderr: %s", err, stderr.String())
	}

	session, err = client.NewSession()
	if err != nil {
		return fmt.Errorf("failed to create session for service start: %v", err)
	}
	defer session.Close()

	cmd = fmt.Sprintf("echo %s | sudo -S systemctl start status-updater", password)
	err = session.Run(cmd)
	if err != nil {
		return fmt.Errorf("failed to start service: %v, stderr: %s", err, stderr.String())
	}

	session, err = client.NewSession()
	if err != nil {
		return fmt.Errorf("failed to create session for status check: %v", err)
	}
	defer session.Close()

	cmd = fmt.Sprintf("echo %s | sudo -S systemctl status status-updater", password)
	err = session.Run(cmd)
	if err != nil {
		return fmt.Errorf("service verification failed - status-updater might not be running: %v", err)
	}

	return nil
}
