package config

type Config struct {
	MQTT struct {
		Broker   string `json:"broker"`
		BrokerIP string `json:"broker_ip"`
		Port     int    `json:"port"`
		ClientID string `json:"client_id"`
		Username string `json:"username"`
		Password string `json:"password"`
	} `json:"mqtt"`
	Log struct {
		Level string `json:"level"`
		File  string `json:"file"`
	} `json:"log"`
	SleepInterval  int `json:"sleep_interval"`
	UpdaterService struct {
		MetadataURL string `json:"metadata_url"`
		Username    string `json:"username"`
		Password    string `json:"password"`
	} `json:"updater_service"`
}

var Current Config

var LogLevels = map[string]int{
	"DEBUG": 1,
	"INFO":  2,
	"WARN":  3,
	"ERROR": 4,
}
