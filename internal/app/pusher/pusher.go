package pusher

import (
	"errors"
	"github.com/sideshow/apns2"
	"github.com/sideshow/apns2/certificate"
	"github.com/sideshow/apns2/payload"
)

type Pusher struct {
	client *apns2.Client
}

func NewPusher(certFile string) (*Pusher, error) {
	cert, err := certificate.FromPemFile(certFile, "")
	if err != nil {
		return nil, err
	}

	return &Pusher{
		client: apns2.NewClient(cert).Development(),
	}, nil
}

func (p *Pusher) Push(devToken, number string) error {
	not := &apns2.Notification{
		DeviceToken: devToken,
		Payload:     payload.NewPayload().Custom("status", "call").Custom("number", number),
		PushType:    apns2.PushTypeVOIP,
	}

	res, err := p.client.Push(not)
	if err != nil {
		return err
	}

	if res.StatusCode != 200 {
		return errors.New(res.Reason)
	}

	return nil
}
