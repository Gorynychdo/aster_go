package ariserver

import (
	"context"

	"github.com/CyCoreSystems/ari/v5"
	"github.com/CyCoreSystems/ari/v5/client/native"
	"github.com/Gorynychdo/aster_go/internal/app/pusher"
	"github.com/Gorynychdo/aster_go/internal/app/store"
	"github.com/inconshreveable/log15"
)

type server struct {
	logger log15.Logger
	config *Config
	client ari.Client
	store  *store.Store
	pusher *pusher.Pusher
	conns  map[string]*connection
}

func newServer(config *Config, store *store.Store) (*server, error) {
	s := &server{
		logger: log15.New(),
		config: config,
		store:  store,
		conns:  make(map[string]*connection),
	}

	s.logger.SetHandler(log15.MultiHandler(
		log15.Must.FileHandler("logs/app.log", log15.LogfmtFormat()),
		log15.StderrHandler),
	)

	var err error
	s.pusher, err = pusher.NewPusher(s.config.CertFile)
	if err != nil {
		return nil, err
	}

	native.Logger = s.logger
	s.client, err = native.Connect(&native.Options{
		Application:  s.config.Application,
		Username:     s.config.Username,
		Password:     s.config.Password,
		URL:          s.config.URL,
		WebsocketURL: s.config.WebsocketURL,
		SubscribeAll: true,
	})
	if err != nil {
		s.logger.Error("Failed to build native ARI client", "error", err)
		return nil, err
	}

	s.logger.Info("Connected")

	return s, nil
}

func (s *server) serve() {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	s.logger.Info("Starting serve")

	sub := s.client.Bus().Subscribe(nil, "StasisStart")
	end := s.client.Bus().Subscribe(nil, "StasisEnd")
	st := s.client.Bus().Subscribe(nil, "ContactStatusChange")

	for {
		select {
		case e := <-sub.Events():
			v := e.(*ari.StasisStart)
			s.logger.Info("Got stasis start", "channel", v.Channel.ID)

			if len(v.Args) == 2 {
				con := newConnection(s, s.client.Channel().Get(v.Key(ari.ChannelKey, v.Channel.ID)), v.Args)
				go con.handle()
			}
		case e := <-end.Events():
			v := e.(*ari.StasisEnd)
			s.logger.Info("Got stasis end", "channel", v.Channel.ID)
		case e := <-st.Events():
			v := e.(*ari.ContactStatusChange)
			s.logger.Info("Contact status changed", "endpoint", v.Endpoint.Resource, "state", v.Endpoint.State)

			con, ok := s.conns[v.Endpoint.Resource]
			if ok && v.Endpoint.State == "online" {
				close(con.connect)
			}
		case <-ctx.Done():
			return
		}
	}
}
