package controller

import (
	"context"
	"reflect"

	v1 "github.com/shopware/shopware-operator/api/v1"
	"github.com/shopware/shopware-operator/internal/event"
	"sigs.k8s.io/controller-runtime/pkg/log"
)

func (c *StoreReconciler) SendEvent(ctx context.Context, store v1.Store, message string) {
	log := log.FromContext(ctx).WithValues(
		"message", message,
		"currentImageTag", store.Status.CurrentImageTag,
		"condition", store.Status.GetLastCondition(),
		"labels", store.Labels,
	)

	e := event.Event{
		Message:       message,
		Condition:     store.Status.GetLastCondition(),
		DeployedImage: store.Status.CurrentImageTag,
		StoreLabels:   store.Labels,
	}

	for _, handler := range c.EventHandlers {
		log.Info("Sending event", "handler", reflect.TypeOf(handler).String())
		err := handler.Send(ctx, e)
		if err != nil {
			log.Error(err, "Sending event", "handler", reflect.TypeOf(handler).String())
		}
	}
}
