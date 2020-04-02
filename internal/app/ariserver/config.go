package ariserver

type Config struct {
	Application  string `toml:"application"`
	Username     string `toml:"username"`
	Password     string `toml:"password"`
	URL          string `toml:"url"`
	WebsocketURL string `toml:"websocket_url"`
	DatabaseURL  string `toml:"database_url"`
	CertFile     string `toml:"cert_file"`
	SpoolPath    string `toml:"spool_path"`
	RecPath      string `toml:"rec_path"`
}

func NewConfig() *Config {
	return &Config{
		Application:  "example",
		Username:     "admin",
		Password:     "admin",
		URL:          "http://localhost:8088/ari",
		WebsocketURL: "ws://localhost:8088/ari/events",
		DatabaseURL:  "host=localhost dbname=test sslmode=disable",
		CertFile:     "configs/cert.pem",
		SpoolPath:    "/var/spool/asterisk/recording",
		RecPath:      "/tmp/records",
	}
}
