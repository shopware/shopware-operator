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
	e := event.Event{
		Message:       message,
		Condition:     store.Status.GetLastCondition(),
		DeployedImage: store.Status.CurrentImageTag,
		Labels:        store.Labels,
		KindType:      reflect.TypeOf(store).String(),
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

func (c *StoreSnapshotCreateReconciler) SendEvent(ctx context.Context, snap v1.StoreSnapshotCreate) {
	e := event.Event{
		Message:       snap.Status.Message,
		Condition:     snap.Status.GetLastCondition(),
		DeployedImage: snap.Spec.Container.Image,
		Labels:        snap.Labels,
		KindType:      reflect.TypeOf(snap).String(),
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
