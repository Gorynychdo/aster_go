package ariserver

func Start(config *Config) error {
	srv := newServer()
	if err := srv.init(config); err != nil {
		return err
	}

	srv.serve()
	return nil
}
