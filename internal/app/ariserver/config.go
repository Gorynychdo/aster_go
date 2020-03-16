package ariserver

type Config struct {
	Application  string `toml:"application"`
	Username     string `toml:"username"`
	Password     string `toml:"password"`
	URL          string `toml:"url"`
	WebsocketURL string `toml:"websocket_url"`
}

func NewConfig() *Config {
	return &Config{
		Application:  "example",
		Username:     "admin",
		Password:     "admin",
		URL:          "http://localhost:8088/ari",
		WebsocketURL: "ws://localhost:8088/ari/events",
	}
}
