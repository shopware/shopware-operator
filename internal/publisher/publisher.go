package publisher

import "context"

type Publisher interface {
	Publish(ctx context.Context, name string, payload interface{}) error
}
