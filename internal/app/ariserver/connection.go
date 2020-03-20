package ariserver

import (
	"github.com/CyCoreSystems/ari/v5"
	"github.com/CyCoreSystems/ari/v5/rid"
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

func newConnection(caller, callee string, ch *ari.ChannelHandle, s *server) *connection {
	return &connection{
		server:        s,
		caller:        caller,
		callee:        callee,
		ch:            make(chan int, 1),
		callerHandler: ch,
	}
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
