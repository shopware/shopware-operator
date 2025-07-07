package nats

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/nats-io/nats.go"
	"github.com/shopware/shopware-operator/internal/event"
)

var _ event.EventHandler = (*NatsEventServer)(nil)

type NatsEventServer struct {
	conn  *nats.Conn
	topic string
}

// Send implements event.EventHandler.
func (w *NatsEventServer) Send(ctx context.Context, event event.Event) error {
	data, err := json.MarshalIndent(event, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal event data in nats handler: %w", err)
	}

	err = w.conn.Publish(w.topic, data)
	if err != nil {
		return fmt.Errorf("failed to publish event to NATS: %w", err)
	}
	return nil
}

func (w *NatsEventServer) Close() {
	w.conn.Close()
}

func NewNatsEventServer(addr string, nkeyFile string, credentialsFile string, topic string) (*NatsEventServer, error) {
	options := []nats.Option{
		nats.Name("shopware-operator"),
	}

	if nkeyFile != "" {
		opt, err := nats.NkeyOptionFromSeed(nkeyFile)
		if err != nil {
			return &NatsEventServer{}, fmt.Errorf("failed to read NKeyFile: %w", err)
		}
		options = append(options, opt)
	}

	if credentialsFile != "" {
		options = append(options, nats.UserCredentials(credentialsFile))
	}

	// Connect to a server
	nc, err := nats.Connect(addr, options...)
	if err != nil {
		return &NatsEventServer{}, fmt.Errorf("failed to connect to NATS server: %w", err)
	}

	if !nc.IsConnected() {
		return &NatsEventServer{}, fmt.Errorf("failed isConnected test NATS server: %w", err)
	}

	return &NatsEventServer{
		conn:  nc,
		topic: topic,
	}, nil
}
