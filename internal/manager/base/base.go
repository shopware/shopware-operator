package base

import (
	"context"
	"fmt"

	v1 "github.com/shopware/shopware-operator/api/v1"
	"github.com/shopware/shopware-operator/internal/deployment"
	"github.com/shopware/shopware-operator/internal/event"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/record"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

const (
	Error = "Error"
	Ready = "Ready"
)

type Base struct {
	client.Client
	Clientset            *kubernetes.Clientset
	RestConfig           *rest.Config
	Scheme               *runtime.Scheme
	Recorder             record.EventRecorder
	EventHandlers        []event.EventHandler
	DisableServiceChecks bool
}

func (b *Base) Eventf(store *v1.Store, reason string, format string, args ...any) {
	if b.Recorder != nil {
		b.Recorder.Event(store, "Normal", reason, fmt.Sprintf(format, args...))
	}
}

func (b *Base) AllDeploymentsRunning(ctx context.Context, store *v1.Store) bool {
	store.Status.AdminState = deployment.GetAdminDeploymentCondition(ctx, *store, b.Client)
	store.Status.WorkerState = deployment.GetWorkerDeploymentCondition(ctx, *store, b.Client)
	store.Status.StorefrontState = deployment.GetStorefrontDeploymentCondition(ctx, *store, b.Client)

	return store.Status.AdminState.State == v1.DeploymentStateRunning &&
		store.Status.WorkerState.State == v1.DeploymentStateRunning &&
		store.Status.StorefrontState.State == v1.DeploymentStateRunning
}
