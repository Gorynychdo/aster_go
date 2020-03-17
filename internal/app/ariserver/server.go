package ariserver

import (
	"context"
	"github.com/BurntSushi/toml"
	"github.com/CyCoreSystems/ari/v5"
	"github.com/CyCoreSystems/ari/v5/client/native"
	"github.com/inconshreveable/log15"
)

type server struct {
	logger log15.Logger
	client ari.Client
}

func NewServer() *server {
	return &server{
		logger: log15.New(),
	}
}

func (s *server) Start(configPath string) error {
	options := &native.Options{}
	_, err := toml.DecodeFile(configPath, options)
	if err != nil {
		s.logger.Error("Failed to read configuration", "error", err)
		return err
	}

	native.Logger = s.logger

	s.client, err = native.Connect(options)
	if err != nil {
		s.logger.Error("Failed to build native ARI client", "error", err)
		return err
	}

	s.logger.Info("Connected")
	return nil
}

func (s *server) Serve() {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	s.logger.Info("Starting listener app")
	s.listenApp(ctx, s.channelHandler)
}

func (s *server) listenApp(
	ctx context.Context,
	handler func(ctx context.Context, h *ari.ChannelHandle, args []string),
	) {
	sub := s.client.Bus().Subscribe(nil, "StasisStart")
	end := s.client.Bus().Subscribe(nil, "StasisEnd")

	for {
		select {
		case e := <-sub.Events():
			v := e.(*ari.StasisStart)
			s.logger.Info("Got stasis start", "channel", v.Channel.ID)
			go handler(ctx, s.client.Channel().Get(v.Key(ari.ChannelKey, v.Channel.ID)), v.Args)
		case <-end.Events():
			s.logger.Info("Got stasis end")
		case <-ctx.Done():
			return
		}
	}
}

func (s *server) channelHandler(ctx context.Context, h *ari.ChannelHandle, args []string) {
	s.logger.Info("Running channel handler")
	s.logger.Info("Caller ID", "id", args[0])
	s.logger.Info("Extension", "ext", args[1])

	if err := h.Answer(); err != nil {
		s.logger.Error("Failed to answer call", "error", err)
	}

	s.logger.Info("Answering to call")
	h.Hangup()
}
