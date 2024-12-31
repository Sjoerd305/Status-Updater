package gatherer

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"status-updater/helpers"
	"status-updater/logger"
	"strconv"
	"strings"
	"time"
)

// Reads Helpcom config files from /opt/helpcom/etc/
func ReadHelpcomConfig() (map[string]string, error) {
	helpcomConfig := make(map[string]string)

	files := map[string]string{
		"/opt/helpcom/etc/servers":  "HelpcomServers",
		"/opt/helpcom/etc/lifespan": "HelpcomLifespan",
		"/opt/helpcom/etc/rf":       "HelpcomRF",
	}

	for path, key := range files {
		if _, err := os.Stat(path); os.IsNotExist(err) {
			helpcomConfig[key] = "N/A"
		} else {
			content, err := os.ReadFile(path)
			if err != nil {
				logger.LogMessage("WARN", fmt.Sprintf("Failed to read %s: %s", path, err))
				helpcomConfig[key] = "N/A"
			} else {
				helpcomConfig[key] = strings.TrimSpace(string(content))
			}
		}
	}

	return helpcomConfig, nil
}

// Returns status of running services based on device type
func GetServiceStatus() string {
	deviceType, err := GetDeviceType()
	if err != nil {
		logger.LogMessage("ERROR", fmt.Sprintf("Failed to determine device type: %v", err))
		return "Unknown"
	}
	var services []string

	sosServices := []string{
		"sos-audio",
		"sos-businesslogicserver",
		"sos-hc100-emu",
		"sos-helpcom",
		"sos-nas",
		"sos-vca",
		"sos-web",
	}

	if deviceType == "hc900" || deviceType == "hc925" || deviceType == "hc950" {
		if helpers.IsBuildroot() {
			status := helpers.CheckInitDServiceStatus("helpcom")
			if status != "" {
				services = append(services, status)
			} else {
				services = append(services, "helpcom: stopped")
			}
		} else {
			status := helpers.CheckServiceStatus("helpcom")
			if status != "" {
				services = append(services, status)
			} else {
				services = append(services, "helpcom: inactive")
			}
		}
	} else {
		if helpers.IsBuildroot() {
			logger.LogMessage("INFO", "Running on Buildroot, skipping SOS service check")
		} else {
			for _, service := range sosServices {
				status := helpers.CheckServiceStatus(service)
				if status != "" {
					services = append(services, status)
				} else {
					services = append(services, fmt.Sprintf("%s: inactive", service))
				}
			}
		}
	}

	if len(services) == 0 {
		return "No services found"
	}

	return strings.Join(services, ", ")
}

// Reads device type from config or defaults to SOS
func GetDeviceType() (string, error) {
	deviceTypeFile := "/opt/helpcom/etc/device-type"
	data, err := os.ReadFile(deviceTypeFile)
	if err != nil {
		if os.IsNotExist(err) {
			cmd := exec.Command("dpkg-query", "--showformat='${Version}'", "--show", "sospi2")
			output, err := cmd.Output()
			if err != nil {
				logger.LogMessage("WARN", fmt.Sprintf("Failed to get SOS version: %s", err))
				return "SOS: Unknown", nil
			}
			sosVersion := strings.Trim(string(output), "'")
			return fmt.Sprintf("SOS: %s", sosVersion), nil
		}
		return "", fmt.Errorf("failed to read device type from file: %v", err)
	}
	deviceType := strings.TrimSpace(string(data))
	if deviceType != "hc900" && deviceType != "hc925" && deviceType != "hc950" {
		return "", fmt.Errorf("unknown device type: %s", deviceType)
	}
	return deviceType, nil
}

// Returns MAC addresses for all network interfaces
func GetMACAddresses() string {
	cmd := exec.Command("ip", "link", "show")
	output, err := cmd.Output()
	if err != nil {
		logger.LogMessage("ERROR", fmt.Sprintf("Failed to get MAC addresses: %s", err))
		return "[]"
	}

	macAddresses := []map[string]string{}
	lines := strings.Split(string(output), "\n")
	var interfaceName string
	for _, line := range lines {
		if strings.Contains(line, ": ") && !strings.Contains(line, "link/") {
			parts := strings.Fields(line)
			if len(parts) >= 2 {
				interfaceName = strings.TrimSuffix(parts[1], ":")
			}
		}
		if strings.Contains(line, "link/ether") {
			parts := strings.Fields(line)
			if len(parts) >= 2 {
				macAddress := parts[1]
				macAddresses = append(macAddresses, map[string]string{
					"interface":   interfaceName,
					"mac_address": macAddress,
				})
				logger.LogMessage("INFO", fmt.Sprintf("Retrieved MAC address for %s: %s", interfaceName, macAddress))
			}
		}
	}

	macAddressesJSON, err := json.Marshal(macAddresses)
	if err != nil {
		logger.LogMessage("ERROR", fmt.Sprintf("Failed to marshal MAC addresses: %s", err))
		return "[]"
	}

	return string(macAddressesJSON)
}

// Returns IP addresses for all network interfaces
func GetIPAddresses() string {
	cmd := exec.Command("ip", "-o", "-4", "addr", "list")
	output, err := cmd.Output()
	if err != nil {
		logger.LogMessage("ERROR", fmt.Sprintf("Failed to get IP addresses: %s", err))
		return "[]"
	}

	ipAddresses := []map[string]string{}
	lines := strings.Split(string(output), "\n")
	for _, line := range lines {
		if line != "" {
			parts := strings.Fields(line)
			if len(parts) >= 4 {
				interfaceName := parts[1]
				ipAddress := strings.Split(parts[3], "/")[0]
				ipAddresses = append(ipAddresses, map[string]string{
					"interface":  interfaceName,
					"ip_address": ipAddress,
				})
				logger.LogMessage("INFO", fmt.Sprintf("Retrieved IP address for %s: %s", interfaceName, ipAddress))
			}
		}
	}

	ipAddressesJSON, err := json.Marshal(ipAddresses)
	if err != nil {
		logger.LogMessage("ERROR", fmt.Sprintf("Failed to marshal IP addresses: %s", err))
		return "[]"
	}

	return string(ipAddressesJSON)
}

// Returns modem details via mmcli
func GetModemDetails() string {
	if _, err := exec.LookPath("mmcli"); err != nil {
		logger.LogMessage("WARN", "mmcli command not found. No modem information will be retrieved.")
		return `{"manufacturer":"N/A","model":"N/A","signal_quality":"N/A","state":"N/A","imei":"N/A","operator_id":"N/A","imsi":"N/A"}`
	}

	cmd := exec.Command("mmcli", "-L")
	output, err := cmd.Output()
	if err != nil {
		logger.LogMessage("WARN", fmt.Sprintf("Failed to get modem list: %s", err))
		return `{"manufacturer":"N/A","model":"N/A","signal_quality":"N/A","state":"N/A","imei":"N/A","operator_id":"N/A","imsi":"N/A"}`
	}

	modemIndex := -1
	lines := strings.Split(strings.TrimSpace(string(output)), "\n")
	for _, line := range lines {
		if strings.HasPrefix(line, "/org/freedesktop/ModemManager1/Modem/") {
			parts := strings.Split(line, " ")
			if len(parts) > 0 {
				indexStr := strings.TrimPrefix(parts[0], "/org/freedesktop/ModemManager1/Modem/")
				if index, err := strconv.Atoi(indexStr); err == nil {
					modemIndex = index
					break
				}
			}
		}
	}

	if modemIndex == -1 {
		logger.LogMessage("WARN", "No modems found")
		return `{"manufacturer":"N/A","model":"N/A","signal_quality":"N/A","state":"N/A","imei":"N/A","operator_id":"N/A","imsi":"N/A"}`
	}

	cmd = exec.Command("mmcli", "-m", strconv.Itoa(modemIndex))
	output, err = cmd.Output()
	if err != nil {
		logger.LogMessage("WARN", fmt.Sprintf("Failed to get modem details: %s", err))
		return `{"manufacturer":"N/A","model":"N/A","signal_quality":"N/A","state":"N/A","imei":"N/A","operator_id":"N/A","imsi":"N/A"}`
	}

	modemInfo := string(output)
	modemManufacturer := helpers.ExtractField(modemInfo, "manufacturer")
	modemModel := helpers.ExtractField(modemInfo, "model")
	modemHWRevision := helpers.ExtractField(modemInfo, "h/w revision")
	modemSignalQuality := helpers.ExtractField(modemInfo, "signal quality")
	modemSignalQuality = helpers.ExtractPercentage(modemSignalQuality)
	modemIMEI := helpers.ExtractField(modemInfo, "imei")
	modemState := helpers.ExtractField(modemInfo, "state")
	modemState = helpers.StripANSI(modemState)

	if strings.Contains(modemManufacturer, "SIMCOM") {
		modemModel = modemHWRevision
	}

	cmd = exec.Command("mmcli", "-i", strconv.Itoa(modemIndex))
	output, err = cmd.Output()
	if err != nil {
		logger.LogMessage("WARN", fmt.Sprintf("Failed to get SIM details: %s", err))
		return `{"manufacturer":"N/A","model":"N/A","signal_quality":"N/A","state":"N/A","imei":"N/A","operator_id":"N/A","imsi":"N/A"}`
	}

	simInfo := string(output)
	modemIMSI := helpers.ExtractField(simInfo, "imsi")
	modemOperatorID := helpers.ExtractField(simInfo, "operator id")
	modemOperator := helpers.ExtractField(simInfo, "operator name")

	modemDetails := map[string]string{
		"manufacturer":   modemManufacturer,
		"model":          modemModel,
		"signal_quality": modemSignalQuality,
		"state":          modemState,
		"imei":           modemIMEI,
		"operator":       modemOperator,
		"operator_id":    modemOperatorID,
		"imsi":           modemIMSI,
	}

	modemDetailsJSON, err := json.Marshal(modemDetails)
	if err != nil {
		logger.LogMessage("ERROR", fmt.Sprintf("Failed to marshal modem details: %s", err))
		return `{"manufacturer":"N/A","model":"N/A","signal_quality":"N/A","state":"N/A","imei":"N/A","operator_id":"N/A","imsi":"N/A"}`
	}

	return string(modemDetailsJSON)
}

// Returns kernel version
func GetLinuxVersion() string {
	cmd := exec.Command("uname", "-r")
	output, err := cmd.Output()
	if err != nil {
		logger.LogMessage("ERROR", fmt.Sprintf("Failed to get Linux version: %s", err))
		return "Unknown"
	}
	return strings.TrimSpace(string(output))
}

// Returns system uptime from /proc/uptime
func GetUptime() string {
	uptimeBytes, err := os.ReadFile("/proc/uptime")
	if err != nil {
		logger.LogMessage("ERROR", fmt.Sprintf("Failed to read uptime: %s", err))
		return "N/A"
	}

	uptimeStr := strings.Fields(string(uptimeBytes))[0]
	uptimeSeconds, err := strconv.ParseFloat(uptimeStr, 64)
	if err != nil {
		logger.LogMessage("ERROR", fmt.Sprintf("Failed to parse uptime: %s", err))
		return "N/A"
	}

	uptimeDuration := time.Duration(uptimeSeconds) * time.Second
	return uptimeDuration.String()
}

// Returns connected AP MAC via iwgetid
func GetAccessPointMAC() string {
	cmd := exec.Command("iwgetid", "-a")
	output, err := cmd.Output()
	if err != nil || strings.TrimSpace(string(output)) == "" {
		logger.LogMessage("INFO", "No Access Point MAC found or failed to get Access Point MAC")
		return "N/A"
	}

	lines := strings.Split(string(output), "\n")
	for _, line := range lines {
		if strings.Contains(line, "Access Point/Cell:") {
			parts := strings.Split(line, ": ")
			if len(parts) == 2 {
				return strings.TrimSpace(parts[1])
			}
		}
	}
	return "N/A"
}

// Returns LLDP neighbor details
func GetLLDPDetails() (string, string, string, string, string, string, string) {
	if _, err := exec.LookPath("lldpcli"); err != nil {
		logger.LogMessage("WARN", "Skipping LLDP information retrieval.")
		return "N/A", "N/A", "N/A", "N/A", "N/A", "N/A", "N/A"
	}

	cmd := exec.Command("lldpcli", "show", "neighbors", "details")
	output, err := cmd.Output()
	if err != nil {
		logger.LogMessage("ERROR", fmt.Sprintf("Failed to get LLDP details: %s", err))
		return "N/A", "N/A", "N/A", "N/A", "N/A", "N/A", "N/A"
	}

	lldpInfo := string(output)
	switchName := helpers.ExtractField(lldpInfo, "SysName")
	switchIP := helpers.ExtractField(lldpInfo, "MgmtIP")
	switchPort := helpers.ExtractField(lldpInfo, "PortID")
	switchMacAddress := helpers.ExtractField(lldpInfo, "ChassisID")
	switchPortVlan := helpers.ExtractField(lldpInfo, "VLAN")
	switchSysDescription := helpers.ExtractField(lldpInfo, "SysDescr")
	switchPortDescription := helpers.ExtractField(lldpInfo, "PortDescr")

	return switchName, switchIP, switchPort, switchMacAddress, switchPortVlan, switchSysDescription, switchPortDescription
}

// Returns CPU/GPU temp from vcgencmd or thermal zone
func GetTemperature() string {
	if helpers.IsBuildroot() {
		logger.LogMessage("INFO", "Running on Buildroot, skipping temperature measurement")
		return "N/A"
	}

	cmd := exec.Command("/opt/vc/bin/vcgencmd", "measure_temp")
	output, err := cmd.Output()
	if err == nil {
		tempOutput := strings.TrimSpace(string(output))
		tempParts := strings.Split(tempOutput, "=")
		if len(tempParts) == 2 {
			return strings.TrimSuffix(tempParts[1], "'C")
		}
	}

	thermalZonePath := "/sys/class/thermal/thermal_zone0/temp"
	tempBytes, err := os.ReadFile(thermalZonePath)
	if err != nil {
		logger.LogMessage("ERROR", fmt.Sprintf("Failed to read temperature from %s: %s", thermalZonePath, err))
		return "N/A"
	}

	tempStr := strings.TrimSpace(string(tempBytes))
	tempInt, err := strconv.Atoi(tempStr)
	if err != nil {
		logger.LogMessage("ERROR", fmt.Sprintf("Failed to parse temperature: %s", err))
		return "N/A"
	}

	return fmt.Sprintf("%.2f", float64(tempInt)/1000.0)
}
