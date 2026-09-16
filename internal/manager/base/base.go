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
	EnableKeda           bool
	OperatorMetricsURL   string
}

func (b *Base) Eventf(store *v1.Store, reason string, format string, args ...any) {
	if b.Recorder != nil {
		b.Recorder.Event(store, "Normal", reason, fmt.Sprintf(format, args...))
	}
}

func (b *Base) refreshDeploymentStates(ctx context.Context, store *v1.Store) []v1.DeploymentState {
	store.Status.AdminState = deployment.GetAdminDeploymentCondition(ctx, *store, b.Client)
	store.Status.WorkerState = deployment.GetWorkerDeploymentCondition(ctx, *store, b.Client, b.EnableKeda)
	store.Status.StorefrontState = deployment.GetStorefrontDeploymentCondition(ctx, *store, b.Client)

	return []v1.DeploymentState{
		store.Status.AdminState.State,
		store.Status.WorkerState.State,
		store.Status.StorefrontState.State,
	}
}

func (b *Base) AllDeploymentsRunning(ctx context.Context, store *v1.Store) bool {
	for _, state := range b.refreshDeploymentStates(ctx, store) {
		if state != v1.DeploymentStateRunning {
			return false
		}
	}
	return true
}

// AllDeploymentsAvailable also accepts deployments that are scaling. Replicas
// are moved by autoscalers and by manual scaling, which must not send a
// running store back to initializing.
func (b *Base) AllDeploymentsAvailable(ctx context.Context, store *v1.Store) bool {
	for _, state := range b.refreshDeploymentStates(ctx, store) {
		if state != v1.DeploymentStateRunning && state != v1.DeploymentStateScaling {
			return false
		}
	}
	return true
}
