package controller

import (
	"context"
	"reflect"

	v1 "github.com/shopware/shopware-operator/api/v1"
	"github.com/shopware/shopware-operator/internal/event"
	"github.com/shopware/shopware-operator/internal/logging"
	"go.uber.org/zap"
)

func (c *StoreReconciler) SendEvent(ctx context.Context, store v1.Store, message string) {
	log := logging.FromContext(ctx).With(
		zap.String("message", message),
		zap.String("currentImageTag", store.Status.CurrentImageTag),
		zap.Any("condition", store.Status.GetLastCondition()),
		zap.Any("labels", store.Labels),
	)

	e := event.Event{
		Message:       message,
		Condition:     store.Status.GetLastCondition(),
		DeployedImage: store.Status.CurrentImageTag,
		Labels:        store.Labels,
	}

	for _, handler := range c.EventHandlers {
		log.Info("Sending event", "handler", reflect.TypeOf(handler).String())
		err := handler.Send(ctx, e)
		if err != nil {
			log.Error(err, "Sending event", "handler", reflect.TypeOf(handler).String())
		}
	}
}
