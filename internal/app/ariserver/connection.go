package ariserver

import (
	"github.com/CyCoreSystems/ari/v5"
	"github.com/CyCoreSystems/ari/v5/rid"
	"sync"
	"time"
)

type connection struct {
	*server
	caller        string
	callee        string
	ch            chan int
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
		ch:            make(chan int, 1),
	}

	c.logger.Info("Calling", "caller", c.caller, "callee", c.callee)
	return c
}

func (c *connection) handle() {
	user, err := c.store.User().Find(c.callee)
	if err != nil {
		c.logger.Error("Filed to find user", "error", err)
		c.close()
		return
	}

	if err := c.pusher.Push(user.DeviceToken, c.caller); err != nil {
		c.logger.Error("Push notification failed", "error", err)
		c.close()
		return
	}

	select {
	case <-c.ch:
		break
	case <-time.After(30 * time.Second):
		c.close()
		return
	}

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
		c.logger.Error("Failed to dialing", "error", err)
		c.close()
		return
	}

	chEnd := c.callerHandler.Subscribe(ari.Events.StasisEnd)
	orEnd := c.calleeHandler.Subscribe(ari.Events.StasisEnd)
	orStart := c.calleeHandler.Subscribe(ari.Events.StasisStart)

	var wg sync.WaitGroup

	wg.Add(1)
	go func() {
		defer wg.Done()

		for {
			select {
			case <-orStart.Events():
				if err := c.callerHandler.Answer(); err != nil {
					c.logger.Error("failed to answer call", "error", err)
					c.close()
					return
				}

				if err := c.createBridge(); err != nil {
					return
				}
			case <-orEnd.Events():
				c.calleeHandler = nil
				c.close()
				return
			case <-chEnd.Events():
				c.callerHandler = nil
				c.close()
				return
			}
		}
	}()

	wg.Wait()
}

func (c *connection) createBridge() (err error) {
	brKey := ari.NewKey(ari.BridgeKey, rid.New(rid.Bridge))
	c.bridge, err = c.client.Bridge().Create(brKey, "mixing", brKey.ID)
	if err != nil {
		c.logger.Error("Failed to create bridge", "error", err)
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
	c.logger.Debug("Connections", "conns", c.conns)
}
