package ariserver

import (
	"context"
	"github.com/CyCoreSystems/ari/v5"
	"github.com/CyCoreSystems/ari/v5/client/native"
	"github.com/CyCoreSystems/ari/v5/ext/play"
	"github.com/Gorynychdo/aster_go/internal/app/pusher"
	"github.com/Gorynychdo/aster_go/internal/app/store"
	"github.com/inconshreveable/log15"
)

type server struct {
	logger log15.Logger
	client ari.Client
	store  *store.Store
	pusher *pusher.Pusher
}

func newServer(config *Config, store *store.Store) (*server, error) {
	s := &server{
		logger: log15.New(),
		store:  store,
	}

	var err error
	s.pusher, err = pusher.NewPusher(config.CertFile)
	if err != nil {
		return nil, err
	}

	native.Logger = s.logger
	s.client, err = native.Connect(&native.Options{
		Application:  config.Application,
		Username:     config.Username,
		Password:     config.Password,
		URL:          config.URL,
		WebsocketURL: config.WebsocketURL,
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
	con := s.client.Bus().Subscribe(nil, "ContactStatusChange")

	for {
		select {
		case e := <-sub.Events():
			v := e.(*ari.StasisStart)
			s.logger.Info("Got stasis start", "channel", v.Channel.ID)
			go s.channelHandler(ctx, s.client.Channel().Get(v.Key(ari.ChannelKey, v.Channel.ID)), v.Args, con)
		case e := <-end.Events():
			v := e.(*ari.StasisEnd)
			s.logger.Info("Got stasis end", "channel", v.Channel.ID)
		case e := <-con.Events():
			v := e.(*ari.ContactStatusChange)
			s.logger.Info("Contact status changed", "endpoint", v.Endpoint.Resource, "state", v.Endpoint.State)
		case <-ctx.Done():
			return
		}
	}
}

func (s *server) channelHandler(ctx context.Context, h *ari.ChannelHandle, args []string, sub ari.Subscription) {
	caller, callee := args[0], args[1]
	s.logger.Info("Calling", "caller", caller, "callee", callee)

	user, err := s.store.User().Find(callee)
	if err != nil {
		s.logger.Error("Filed to find user", "error", err)
		h.Hangup()
		return
	}

	if err := s.pusher.Push(user.DeviceToken, caller); err != nil {
		s.logger.Error("Push notification failed", "error", err)
	}

	if err := h.Answer(); err != nil {
		s.logger.Error("failed to answer call", "error", err)
		h.Hangup()
		return
	}

	s.logger.Info("Answering to call")

	if err := play.Play(ctx, h, play.URI("sound:tt-monkeys")).Err(); err != nil {
		s.logger.Error("failed to play sound", "error", err)
		return
	}

	h.Hangup()
}
