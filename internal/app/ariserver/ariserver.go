package ariserver

import (
	"database/sql"
	"github.com/Gorynychdo/aster_go/internal/app/store"
	_ "github.com/go-sql-driver/mysql"
	"github.com/sideshow/apns2"
	"github.com/sideshow/apns2/certificate"
)

func Start(config *Config) error {
	db, err := newDB(config.DatabaseURL)
	if err != nil {
		return err
	}

	defer db.Close()
	store := store.New(db)

	pusher, err := newPusher(config.CertFile)
	if err != nil {
		return err
	}

	srv, err := newServer(config, store, pusher)
	if err != nil {
		return err
	}

	srv.serve()
	return nil
}

func newDB(databaseURL string) (*sql.DB, error) {
	db, err := sql.Open("mysql", databaseURL)
	if err != nil {
		return nil, err
	}

	if err = db.Ping(); err != nil {
		return nil, err
	}

	return db, nil
}

func newPusher(certFile string) (*apns2.Client, error) {
	cert, err := certificate.FromPemFile(certFile, "")
	if err != nil {
		return nil, err
	}

	return apns2.NewClient(cert).Production(), nil
}
