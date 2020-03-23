package ariserver

import (
	"context"
	"errors"
	"github.com/CyCoreSystems/ari/v5"
	"github.com/CyCoreSystems/ari/v5/rid"
	"sync"
	"time"
)

type connection struct {
	*server
	caller        string
	callee        string
	ch            chan bool
	callerHandler *ari.ChannelHandle
	calleeHandler *ari.ChannelHandle
	bridge        *ari.BridgeHandle
}

func newConnection(s *server, ch *ari.ChannelHandle, args []string) *connection {
	c := &connection{
		server:        s,
		callerHandler: ch,
		caller:        args[0],
		callee:        args[1],
		ch:            make(chan bool, 1),
	}

	c.logger.Info("Calling", "channel", c.callerHandler.ID(), "caller", c.caller, "callee", c.callee)
	c.conns[c.callee] = c
	return c
}

func (c *connection) handle() {
	defer c.logger.Debug("Leave handler")

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	var wg sync.WaitGroup
	defer wg.Wait()

	end := c.callerHandler.Subscribe(ari.Events.StasisEnd)

	wg.Add(1)
	go func() {
		defer wg.Done()

		select {
		case <-ctx.Done():
			return
		case <-end.Events():
			c.callerHandler = nil
			c.close()
			cancel()
			return
		}
	}()

	success := make(chan bool, 1)
	fail := make(chan bool, 1)

	wg.Add(1)
	go c.callEndpoint(ctx, &wg, success, fail)

	select {
	case <-success:
		break
	case <-fail:
		return
	}

	if err := c.dial(); err != nil {
		return
	}

	calleeStart := c.calleeHandler.Subscribe(ari.Events.StasisStart)
	calleeEnd := c.calleeHandler.Subscribe(ari.Events.StasisEnd)

	wg.Add(1)
	go func() {
		defer wg.Done()

		for {
			select {
			case <-ctx.Done():
				return
			case <-calleeStart.Events():
				if err := c.callerHandler.Answer(); err != nil {
					c.logger.Error("failed to answer call", "channel", c.callerHandler.ID(), "error", err)
					c.close()
					return
				}

				if err := c.createBridge(); err != nil {
					return
				}
			case <-calleeEnd.Events():
				c.calleeHandler = nil
				c.close()
				return
			}
		}
	}()
}

func (c *connection) callEndpoint(ctx context.Context, wg *sync.WaitGroup, success, fail chan<- bool) {
	defer wg.Done()
	defer close(success)
	defer close(fail)

	epReady, err := c.checkEndpoint()
	if err != nil {
		fail <- true
		return
	}

	if epReady {
		success <- true
		return
	}

	select {
	case <-ctx.Done():
		fail <- true
		return
	case <-c.ch:
		success <- true
		return
	case <-time.After(30 * time.Second):
		c.close()
		c.logger.Info("Endpoint calling timeout", "endpoint", c.callee)
		fail <- true
		return
	}
}

func (c *connection) checkEndpoint() (epReady bool, err error) {
	epReady = false
	ep := c.client.Endpoint()
	eph := ep.Get(ari.NewKey(ari.EndpointKey, ari.EndpointID("PJSIP", c.callee)))

	data, err := eph.Data()
	if err != nil {
		c.logger.Error("Failed to get endpoint state", "endpoint", c.callee, "error", err)
		c.close()
		return
	}

	c.logger.Info("Callee endpoint data", "endpoint", data)

	if data.State == "online" {
		if len(data.ChannelIDs) > 0 {
			c.logger.Info("Endpoint is busy", "endpoint", c.callee)

			if err := c.callerHandler.Busy(); err != nil {
				c.logger.Error("Failed to send busy", "channel", c.callerHandler.ID(), "error", err)
				c.close()
			} else {
				c.callerHandler = nil
				c.close()
			}

			err = errors.New("endpoint is busy")
		} else {
			epReady = true
		}
		return
	}

	user, err := c.store.User().Find(c.callee)
	if err != nil {
		c.logger.Error("Filed to find user", "error", err)
		c.close()
		return
	}

	if err = c.pusher.Push(user.DeviceToken, c.caller); err != nil {
		c.logger.Error("Push notification failed", "error", err)
		c.close()
		return
	}

	return
}

func (c *connection) dial() (err error) {
	c.calleeHandler, err = c.callerHandler.Originate(ari.OriginateRequest{
		Endpoint: ari.EndpointID("PJSIP", c.callee),
		Timeout:  30,
		App:      c.config.Application,
		Variables: map[string]string{
			"direct_media":    "no",
			"force_rport":     "yes",
			"rewrite_contact": "yes",
			"rtp_symmetric":   "yes",
		},
	})
	if err != nil {
		c.logger.Error("Failed to dialing", "channel", c.callerHandler.ID(), "error", err)
		c.close()
	}

	return
}

func (c *connection) createBridge() (err error) {
	brKey := ari.NewKey(ari.BridgeKey, rid.New(rid.Bridge))
	c.bridge, err = c.client.Bridge().Create(brKey, "mixing", brKey.ID)
	if err != nil {
		c.logger.Error("Failed to create bridge", "channel", c.callerHandler.ID(), "error", err)
		c.close()
		return
	}

	if err = c.bridge.AddChannel(c.callerHandler.ID()); err != nil {
		c.logger.Error("Failed to add channel to bridge", "channel", c.callerHandler.ID(), "error", err)
		c.close()
		return
	}

	if err = c.bridge.AddChannel(c.calleeHandler.ID()); err != nil {
		c.logger.Error("Failed to add channel to bridge", "channel", c.calleeHandler.ID(), "error", err)
		c.close()
		return
	}

	c.logger.Info("Bridge created", "bridge", c.bridge.ID())
	return
}

func (c *connection) close() {
	if c.bridge != nil {
		if err := c.bridge.Delete(); err != nil {
			c.logger.Error("Bridge destroy failed", "bridge", c.bridge.ID(), "error", err)
		}
		c.bridge = nil
	}

	if c.callerHandler != nil {
		if err := c.callerHandler.Hangup(); err != nil {
			c.logger.Error("Caller hangup failed", "channel", c.callerHandler.ID(), "error", err)
		}
	}

	if c.calleeHandler != nil {
		if err := c.calleeHandler.Hangup(); err != nil {
			c.logger.Error("Callee hangup failed", "channel", c.calleeHandler.ID(), "error", err)
		}
	}

	delete(c.conns, c.callee)
}
