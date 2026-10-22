package controller

import (
	"context"
	"reflect"

	v1 "github.com/shopware/shopware-operator/api/v1"
	"github.com/shopware/shopware-operator/internal/event"
	"github.com/shopware/shopware-operator/internal/logging"
	"github.com/shopware/shopware-operator/internal/tracing"
	"go.uber.org/zap"
)

func (c *StoreSnapshotCreateReconciler) SendEvent(ctx context.Context, snap v1.StoreSnapshotCreate) {
	e := event.Event{
		Message:       snap.Status.Message,
		Condition:     snap.Status.GetLastCondition(),
		DeployedImage: snap.Spec.Container.Image,
		Labels:        snap.Labels,
		KindType:      reflect.TypeOf(snap).String(),
		TraceID:       tracing.TraceID(ctx),
	}

	log := logging.FromContext(ctx).With(
		zap.Any("event", e),
	)

	for _, handler := range c.EventHandlers {
		log.Info("Sending event", "handler", reflect.TypeOf(handler).String())
		err := handler.Send(ctx, e)
		if err != nil {
			log.Error(err, "Sending event", "handler", reflect.TypeOf(handler).String())
		}
	}
}

func (c *StoreSnapshotRestoreReconciler) SendEvent(ctx context.Context, snap v1.StoreSnapshotRestore) {
	e := event.Event{
		Message:       snap.Status.Message,
		Condition:     snap.Status.GetLastCondition(),
		DeployedImage: snap.Spec.Container.Image,
		Labels:        snap.Labels,
		KindType:      reflect.TypeOf(snap).String(),
		TraceID:       tracing.TraceID(ctx),
	}

	log := logging.FromContext(ctx).With(
		zap.Any("event", e),
	)

	for _, handler := range c.EventHandlers {
		log.Info("Sending event", "handler", reflect.TypeOf(handler).String())
		err := handler.Send(ctx, e)
		if err != nil {
			log.Error(err, "Sending event", "handler", reflect.TypeOf(handler).String())
		}
	}
}
