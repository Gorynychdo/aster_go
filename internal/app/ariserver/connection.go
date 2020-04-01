package ariserver

import (
	"context"
	"errors"
	"fmt"
	"path"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/CyCoreSystems/ari/v5"
	"github.com/CyCoreSystems/ari/v5/rid"
)

var (
	errCancelled   = errors.New("cancelled")
	errCallTimeout = errors.New("endpoint call timeout")
	errBusy        = errors.New("endpoint is busy")
)

type connection struct {
	*server
	caller        string
	callee        string
	calleeToken   string
	connect       chan struct{}
	callerHandler *ari.ChannelHandle
	calleeHandler *ari.ChannelHandle
	bridge        *ari.BridgeHandle
	callerRec     *ari.LiveRecordingHandle
	calleeRec     *ari.LiveRecordingHandle
}

func newConnection(s *server, ch *ari.ChannelHandle, args []string) *connection {
	c := &connection{
		server:        s,
		callerHandler: ch,
		caller:        args[0],
		callee:        args[1],
		connect:       make(chan struct{}),
	}

	c.logger.Info("Calling", "channel", c.callerHandler.ID(), "caller", c.caller, "callee", c.callee)
	c.conns[c.callee] = c
	return c
}

func (c *connection) handle() {
	var (
		wg          sync.WaitGroup
		ctx, cancel = context.WithCancel(context.Background())
		end         = c.callerHandler.Subscribe(ari.Events.StasisEnd)
		early       = make(chan struct{})
		callErr     = make(chan error, 1)
	)

	defer func() {
		wg.Wait()
		cancel()
		c.close()
		close(callErr)
		c.logger.Debug("Leave handler")
	}()

	wg.Add(1)
	go func() {
		defer wg.Done()

		select {
		case <-ctx.Done():
			return
		case <-early:
			return
		case <-end.Events():
			end.Cancel()
			c.callerHandler = nil
			cancel()
			return
		}
	}()

	wg.Add(1)
	go c.callEndpoint(ctx, &wg, callErr)

	if err := <-callErr; err != nil {
		switch err {
		case errCancelled:
			c.pushCancel()
		case errCallTimeout:
			c.pushCancel()
			fallthrough
		case errBusy:
			c.logger.Info("Call endpoint failed", "channel", c.callerHandler.ID(), "reason", err)
		default:
			c.logger.Error("Call endpoint failed", "channel", c.callerHandler.ID(), "error", err)
		}
		cancel()
		return
	}

	close(early)

	if err := c.dial(); err != nil {
		c.logger.Error("Failed to dialing", "channel", c.callerHandler.ID(), "error", err)
		return
	}

	calleeStart := c.calleeHandler.Subscribe(ari.Events.StasisStart)
	calleeEnd := c.calleeHandler.Subscribe(ari.Events.StasisEnd)
	hangup := c.calleeHandler.Subscribe(ari.Events.ChannelHangupRequest)

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
					return
				}

				if err := c.createBridge(); err != nil {
					c.logger.Error("Failed to create bridge", "channel", c.callerHandler.ID(), err)
					return
				}
			case <-calleeEnd.Events():
				calleeEnd.Cancel()
				c.calleeHandler = nil
				return
			case <-hangup.Events():
				c.logger.Info("Callee sent hangup", "channel", c.callerHandler.ID(), "callee", c.callee)
				hangup.Cancel()
				c.calleeHandler = nil
				return
			case <-end.Events():
				end.Cancel()
				c.callerHandler = nil
				return
			}
		}
	}()
}

func (c *connection) callEndpoint(ctx context.Context, wg *sync.WaitGroup, errCh chan<- error) {
	defer wg.Done()

	ready, err := c.checkEndpoint()
	if err != nil {
		errCh <- err
		return
	}

	if ready {
		errCh <- nil
		return
	}

	select {
	case <-ctx.Done():
		errCh <- errCancelled
		return
	case <-c.connect:
		errCh <- nil
		return
	case <-time.After(10 * time.Second):
		errCh <- errCallTimeout
		return
	}
}

func (c *connection) checkEndpoint() (bool, error) {
	eph := c.client.Endpoint().Get(ari.NewKey(ari.EndpointKey, ari.EndpointID("PJSIP", c.callee)))

	data, err := eph.Data()
	if err != nil {
		return false, fmt.Errorf("failed to get endpoint state: %v", err)
	}

	c.logger.Info("Callee endpoint data", "endpoint", data)

	if data.State == "online" {
		if len(data.ChannelIDs) > 0 {
			return false, errBusy
		}
		return true, nil
	}

	// user, err := c.store.User().Find(c.callee)
	// if err != nil {
	// 	return false, fmt.Errorf("failed to find user: %v", err)
	// }

	// if err = c.pusher.Push(user.DeviceToken, c.caller, "call"); err != nil {
	// 	return false, fmt.Errorf("failed to push calling: %v", err)
	// }

	// c.calleeToken = user.DeviceToken
	return false, nil
}

func (c *connection) pushCancel() {
	c.logger.Debug("Push cancel")
	if c.calleeToken == "" {
		return
	}

	if err := c.pusher.Push(c.calleeToken, c.caller, "cancel"); err != nil {
		c.logger.Error("Failed to push cancel", "channel", c.callerHandler.ID(), "error", err)
	}
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
	return
}

func (c *connection) createBridge() error {
	var (
		err     error
		brKey   = ari.NewKey(ari.BridgeKey, rid.New(rid.Bridge))
		now     = strconv.FormatInt(time.Now().UnixNano(), 10)
		recOpts = &ari.RecordingOptions{
			Format: "wav",
			Exists: "overwrite",
		}
	)

	c.bridge, err = c.client.Bridge().Create(brKey, "mixing", brKey.ID)
	if err != nil {
		return err
	}

	if err = c.bridge.AddChannel(c.callerHandler.ID()); err != nil {
		return fmt.Errorf("failed to add caller to bridge: %v", err)
	}

	if err = c.bridge.AddChannel(c.calleeHandler.ID()); err != nil {
		return fmt.Errorf("failed to add callee to bridge: %v", err)
	}

	c.logger.Info("Bridge created", "channel", c.callerHandler.ID(), "bridge", c.bridge.ID())


	c.callerRec, err = c.bridge.Record(path.Join(c.caller, now), recOpts)
	if err != nil {
		return fmt.Errorf("failed to create caller recorder: %v", err)
	}

	c.logger.Info("Caller recorder created", "bridge", c.bridge.ID(), "recorder", c.callerRec.ID())

	c.calleeRec, err = c.bridge.Record(path.Join(strings.Replace(c.callee, "int_", "", 1), now), recOpts)
	if err != nil {
		return fmt.Errorf("failed to create callee recorder: %v", err)
	}

	c.logger.Info("Callee recorder created", "bridge", c.bridge.ID(), "recorder", c.calleeRec.ID())

	return nil
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
