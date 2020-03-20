package ariserver

import (
	"context"
	"github.com/CyCoreSystems/ari/v5"
	"github.com/CyCoreSystems/ari/v5/client/native"
	"github.com/Gorynychdo/aster_go/internal/app/pusher"
	"github.com/Gorynychdo/aster_go/internal/app/store"
	"github.com/inconshreveable/log15"
	"sync"
	"time"
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
				caller, callee := v.Args[0], v.Args[1]
				s.logger.Info("Calling", "caller", caller, "callee", callee)
				s.conns[callee] = newConnection(caller, callee, s.client.Channel().Get(v.Key(ari.ChannelKey, v.Channel.ID)), s)
				s.logger.Debug("Connections", "conns", s.conns)

				go s.channelHandler(s.conns[callee])
			}
		case e := <-end.Events():
			v := e.(*ari.StasisEnd)
			s.logger.Info("Got stasis end", "channel", v.Channel.ID)
		case e := <-st.Events():
			v := e.(*ari.ContactStatusChange)
			s.logger.Info("Contact status changed", "endpoint", v.Endpoint.Resource, "state", v.Endpoint.State)

			con, ok := s.conns[v.Endpoint.Resource]
			if ok && v.Endpoint.State == "online" {
				con.ch <- 1
				close(con.ch)
			}
		case <-ctx.Done():
			return
		}
	}
}

func (s *server) channelHandler(conn *connection) {
	user, err := s.store.User().Find(conn.callee)
	if err != nil {
		s.logger.Error("Filed to find user", "error", err)
		conn.close()
		return
	}

	if err := s.pusher.Push(user.DeviceToken, conn.caller); err != nil {
		s.logger.Error("Push notification failed", "error", err)
		conn.close()
		return
	}

	select {
	case <-conn.ch:
		break
	case <-time.After(30 * time.Second):
		conn.close()
		return
	}

	conn.calleeHandler, err = conn.callerHandler.Originate(ari.OriginateRequest{
		Endpoint: ari.EndpointID("PJSIP", conn.callee),
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
		conn.close()
		return
	}

	chEnd := conn.callerHandler.Subscribe(ari.Events.StasisEnd)
	orEnd := conn.calleeHandler.Subscribe(ari.Events.StasisEnd)
	orStart := conn.calleeHandler.Subscribe(ari.Events.StasisStart)

	var wg sync.WaitGroup

	wg.Add(1)
	go func() {
		defer wg.Done()

		for {
			select {
			case <-orStart.Events():
				if err := conn.callerHandler.Answer(); err != nil {
					s.logger.Error("failed to answer call", "error", err)
					conn.close()
					return
				}

				if err := conn.createBridge(); err != nil {
					return
				}
			case <-orEnd.Events():
				conn.calleeHandler = nil
				conn.close()
				return
			case <-chEnd.Events():
				conn.callerHandler = nil
				conn.close()
				return
			}
		}
	}()

	wg.Wait()
}
