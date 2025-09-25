package event

import (
	"context"

	v1 "github.com/shopware/shopware-operator/api/v1"
)

type Event struct {
	// Kind Type {store, snapshotCreate, snapshotRestore, exec, debugInstance}
	KindType string `json:"kindType"`
	// Custom Message
	Message string `json:"message"`
	// Last condition of the store
	Condition v1.StoreCondition `json:"condition"`
	// Current Running image tag
	DeployedImage string `json:"deployedImage"`
	// Labels of the store custom resource
	Labels map[string]string `json:"storeLabels"`
}

type EventHandler interface {
	Send(ctx context.Context, event Event) error
	Close()
}
