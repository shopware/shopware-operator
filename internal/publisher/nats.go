package publisher

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/nats-io/nats.go"
)

// NatsPublisher implements the Publisher interface for NATS.
type NatsPublisher struct {
	conn    *nats.Conn
	subject string
	cluster string
}

// NewNatsPublisher creates a new NatsPublisher.
func NewNatsPublisher(url, subject, cluster string) (*NatsPublisher, error) {
	nc, err := nats.Connect(url)
	if err != nil {
		return nil, fmt.Errorf("failed to connect to nats: %w", err)
	}

	return &NatsPublisher{
		conn:    nc,
		subject: subject,
		cluster: cluster,
	}, nil
}

// Publish sends a message to a NATS subject.
func (p *NatsPublisher) Publish(ctx context.Context, name string, payload map[string]string) error {
	if p.conn == nil {
		return nil // Or return an error indicating not connected
	}

	message := map[string]interface{}{
		"name":      name,
		"component": "store-operator",
		"cluster":   p.cluster,
		"timestamp": time.Now(),
		"payload":   payload,
	}

	data, err := json.Marshal(message)
	if err != nil {
		return fmt.Errorf("failed to marshal nats message: %w", err)
	}

	return p.conn.Publish(p.subject, data)
}

// Close closes the NATS connection.
func (p *NatsPublisher) Close() {
	if p.conn != nil {
		p.conn.Close()
	}
}
