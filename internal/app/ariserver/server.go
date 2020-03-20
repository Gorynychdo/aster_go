package ariserver

import (
	"context"
	"github.com/CyCoreSystems/ari/v5"
	"github.com/CyCoreSystems/ari/v5/client/native"
	"github.com/CyCoreSystems/ari/v5/rid"
	"github.com/Gorynychdo/aster_go/internal/app/pusher"
	"github.com/Gorynychdo/aster_go/internal/app/store"
	"github.com/inconshreveable/log15"
	"sync"
)

type server struct {
	logger log15.Logger
	config *Config
	client ari.Client
	store  *store.Store
	pusher *pusher.Pusher
}

func newServer(config *Config, store *store.Store) (*server, error) {
	s := &server{
		logger: log15.New(),
		config: config,
		store:  store,
	}

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
	con := s.client.Bus().Subscribe(nil, "ContactStatusChange")

	for {
		select {
		case e := <-sub.Events():
			v := e.(*ari.StasisStart)
			s.logger.Info("Got stasis start", "channel", v.Channel.ID)

			if len(v.Args) == 2 {
				go s.channelHandler(s.client.Channel().Get(v.Key(ari.ChannelKey, v.Channel.ID)), v.Args)
			}
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

func (s *server) channelHandler(ch *ari.ChannelHandle, args []string) {
	caller, callee := args[0], args[1]

	s.logger.Info("Calling", "caller", caller, "callee", callee)

	user, err := s.store.User().Find(callee)
	if err != nil {
		s.logger.Error("Filed to find user", "error", err)
		s.safeHangup(ch)
		return
	}

	if err := s.pusher.Push(user.DeviceToken, caller); err != nil {
		s.logger.Error("Push notification failed", "error", err)
		s.safeHangup(ch)
		return
	}

	or, err := ch.Originate(ari.OriginateRequest{
		Endpoint: ari.EndpointID("PJSIP", callee),
		Timeout:  30,
		App:      s.config.Application,
		Variables: map[string]string{
			"direct_media":    "no",
			"force_rport":     "yes",
			"rewrite_contact": "yes",
			"rtp_symmetric":   "yes",
		},
	})
	if err != nil {
		s.logger.Error("Failed to dialing", "error", err)
		s.safeHangup(ch)
		return
	}

	chEnd := ch.Subscribe(ari.Events.StasisEnd)
	orEnd := or.Subscribe(ari.Events.StasisEnd)
	orStart := or.Subscribe(ari.Events.StasisStart)

	var br *ari.BridgeHandle
	var wg sync.WaitGroup

	wg.Add(1)
	go func() {
		defer wg.Done()

		for {
			select {
			case <-orStart.Events():
				if err := ch.Answer(); err != nil {
					s.logger.Error("failed to answer call", "error", err)
					s.safeHangup(ch)
					return
				}

				brKey := ari.NewKey(ari.BridgeKey, rid.New(rid.Bridge))
				br, err = s.client.Bridge().Create(brKey, "mixing", brKey.ID)
				if err != nil {
					s.logger.Error("Failed to create bridge", "error", err)
					s.safeHangup(ch)
					return
				}

				if err := br.AddChannel(ch.ID()); err != nil {
					s.logger.Error("Failed to add channel to bridge", "channel", ch.ID(), "error", err)
					s.safeBridgeDestroy(br)
					s.safeHangup(ch)
					return
				}

				if err := br.AddChannel(or.ID()); err != nil {
					s.logger.Error("Failed to add channel to bridge", "channel", or.ID(), "error", err)
					s.safeBridgeDestroy(br)
					s.safeHangup(or)
					return
				}
			case <-orEnd.Events():
				s.safeBridgeDestroy(br)
				s.safeHangup(ch)
				return
			case <-chEnd.Events():
				s.safeBridgeDestroy(br)
				s.safeHangup(or)
				return
			}
		}
	}()

	wg.Wait()
}

func (s *server) safeHangup(ch *ari.ChannelHandle) {
	if err := ch.Hangup(); err != nil {
		s.logger.Error("Failed to hangup channel", "error", err)
	}
}

func (s *server) safeBridgeDestroy(br *ari.BridgeHandle) {
	if br == nil {
		return
	}

	if err := br.Delete(); err != nil {
		s.logger.Error("Failed to destroy bridge", "error", err)
	}
}
