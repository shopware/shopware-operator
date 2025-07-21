package publisher

import "context"

type Publisher interface {
	Publish(ctx context.Context, name string, payload map[string]string) error
}
