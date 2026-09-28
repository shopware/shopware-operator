package controller

import (
	"context"
	"testing"

	kedav1alpha1 "github.com/kedacore/keda/v2/apis/keda/v1alpha1"
	shopv1 "github.com/shopware/shopware-operator/api/v1"
	"github.com/shopware/shopware-operator/internal/deployment"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/tools/record"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	gatewayv1 "sigs.k8s.io/gateway-api/apis/v1"
)

func storeReconcileTestScheme(t *testing.T) *runtime.Scheme {
	t.Helper()
	scheme := runtime.NewScheme()
	require.NoError(t, clientgoscheme.AddToScheme(scheme))
	require.NoError(t, shopv1.AddToScheme(scheme))
	require.NoError(t, gatewayv1.Install(scheme))
	require.NoError(t, kedav1alpha1.AddToScheme(scheme))
	return scheme
}

func storeForReconcile() *shopv1.Store {
	return &shopv1.Store{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-store",
			Namespace: "test",
		},
		Spec: shopv1.StoreSpec{
			SecretName: "store-secret",
			Container: shopv1.ContainerSpec{
				Image:    "shopware:6.7.0",
				Replicas: 1,
			},
			AdminCredentials: shopv1.Credentials{
				Username: "admin",
			},
			Database: shopv1.DatabaseSpec{
				Host: "mysql",
				Port: 3306,
				User: "shopware",
				Name: "shopware",
				PasswordSecretRef: shopv1.SecretRef{
					Name: "db-secret",
					Key:  "password",
				},
			},
		},
	}
}

func runningStoreDeployments(store *shopv1.Store) []client.Object {
	workers, _ := deployment.WorkerDeployments(*store)
	all := make([]*appsv1.Deployment, 0, 2+len(workers))
	all = append(all, deployment.StorefrontDeployment(*store), deployment.AdminDeployment(*store))
	all = append(all, workers...)
	objs := make([]client.Object, 0, len(all))
	for _, d := range all {
		d.Status = appsv1.DeploymentStatus{
			Replicas:          1,
			AvailableReplicas: 1,
		}
		objs = append(objs, d)
	}
	return objs
}

// A reconcile handles resources for the state the store is in when it starts and only then advances
// the state. A store that changes state during the reconcile therefore still owes a resource pass for
// its new state, so the reconcile must requeue shortly instead of falling through to the long requeue.
func TestReconcileRequeuesShortlyAfterStateTransition(t *testing.T) {
	ctx := context.Background()
	scheme := storeReconcileTestScheme(t)

	store := storeForReconcile()
	store.Status.State = shopv1.StateInitializing
	store.Status.QueueState.Transports = []shopv1.QueueTransportStats{{Name: "async"}}

	dbSecret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{Name: "db-secret", Namespace: "test"},
		Data:       map[string][]byte{"password": []byte("secret")},
	}
	objs := append([]client.Object{store, dbSecret}, runningStoreDeployments(store)...)

	cl := fake.NewClientBuilder().
		WithScheme(scheme).
		WithObjects(objs...).
		WithStatusSubresource(&shopv1.Store{}, &appsv1.Deployment{}).
		Build()

	reconciler := &StoreReconciler{
		Client:   cl,
		Scheme:   scheme,
		Recorder: record.NewFakeRecorder(100),
		Logger:   zap.NewNop().Sugar(),
	}

	result, err := reconciler.Reconcile(ctx, ctrl.Request{
		NamespacedName: types.NamespacedName{Namespace: "test", Name: "test-store"},
	})
	require.NoError(t, err)

	got := &shopv1.Store{}
	require.NoError(t, cl.Get(ctx, types.NamespacedName{Namespace: "test", Name: "test-store"}, got))
	require.Equal(t, shopv1.StateReady, got.Status.State, "precondition: the store changes state in this reconcile")

	assert.Equal(t, shortRequeue, result)
}
