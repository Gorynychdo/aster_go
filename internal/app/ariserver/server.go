package ariserver

import (
	"context"
	"github.com/CyCoreSystems/ari/v5"
	"github.com/CyCoreSystems/ari/v5/client/native"
	"github.com/Gorynychdo/aster_go/internal/app/store"
	"github.com/inconshreveable/log15"
)

type server struct {
	logger log15.Logger
	client ari.Client
	store  *store.Store
}

func newServer(config *Config, store *store.Store) (*server, error) {
	s := &server{
		logger: log15.New(),
		store:  store,
	}

	native.Logger = s.logger
	var err error

	s.client, err = native.Connect(&native.Options{
		Application:  config.Application,
		Username:     config.Username,
		Password:     config.Password,
		URL:          config.URL,
		WebsocketURL: config.WebsocketURL,
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

	for {
		select {
		case e := <-sub.Events():
			v := e.(*ari.StasisStart)
			s.logger.Info("Got stasis start", "channel", v.Channel.ID)
			go s.channelHandler(ctx, s.client.Channel().Get(v.Key(ari.ChannelKey, v.Channel.ID)), v.Args)
		case <-end.Events():
			s.logger.Info("Got stasis end")
		case <-ctx.Done():
			return
		}
	}
}

func (s *server) channelHandler(ctx context.Context, h *ari.ChannelHandle, args []string) {
	user, err := s.store.User().Find(args[1])
	if err != nil {
		s.logger.Error("Filed to find user", "error", err)
		h.Hangup()
		return
	}

	s.logger.Info("User device token", "dev_token", user.DeviceToken)
	s.logger.Info("Answering to call")
	h.Hangup()
}
