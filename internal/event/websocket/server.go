package websocket

import (
	"context"

	"github.com/shopware/shopware-operator/internal/event"
)

var _ event.EventHandler = (*WebsocketEventServer)(nil)

type WebsocketEventServer struct{}

// Close implements event.EventHandler.
func (w *WebsocketEventServer) Close() {
	panic("unimplemented")
}

// Send implements event.EventHandler.
func (w *WebsocketEventServer) Send(ctx context.Context, event event.Event) error {
	panic("unimplemented")
}

func NewEventServer() *WebsocketEventServer {
	return &WebsocketEventServer{}
}
