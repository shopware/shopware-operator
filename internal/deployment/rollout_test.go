package deployment

import (
	"context"
	"testing"

	v1 "github.com/shopware/shopware-operator/api/v1"
	"github.com/stretchr/testify/require"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/utils/ptr"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

func finishedDeployment() *appsv1.Deployment {
	return &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{Generation: 2},
		Spec:       appsv1.DeploymentSpec{Replicas: ptr.To[int32](2)},
		Status: appsv1.DeploymentStatus{
			ObservedGeneration: 2,
			Replicas:           2,
			UpdatedReplicas:    2,
			AvailableReplicas:  2,
		},
	}
}

func TestIsRollingOut(t *testing.T) {
	tests := []struct {
		name   string
		modify func(d *appsv1.Deployment)
		want   bool
	}{
		{
			name:   "finished",
			modify: func(*appsv1.Deployment) {},
			want:   false,
		},
		{
			name: "spec not observed yet",
			modify: func(d *appsv1.Deployment) {
				d.Generation = 3
			},
			want: true,
		},
		{
			name: "not all replicas updated",
			modify: func(d *appsv1.Deployment) {
				d.Status.UpdatedReplicas = 1
			},
			want: true,
		},
		{
			name: "old replicas still terminating",
			modify: func(d *appsv1.Deployment) {
				d.Status.Replicas = 3
			},
			want: true,
		},
		{
			name: "updated replicas not available",
			modify: func(d *appsv1.Deployment) {
				d.Status.AvailableReplicas = 1
			},
			want: true,
		},
		{
			name: "progress deadline exceeded",
			modify: func(d *appsv1.Deployment) {
				d.Status.UpdatedReplicas = 1
				d.Status.Conditions = []appsv1.DeploymentCondition{{
					Type:   appsv1.DeploymentProgressing,
					Status: corev1.ConditionFalse,
					Reason: "ProgressDeadlineExceeded",
				}}
			},
			want: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			d := finishedDeployment()
			tt.modify(d)
			require.Equal(t, tt.want, isRollingOut(d))
		})
	}
}

func TestRolloutInProgress(t *testing.T) {
	scheme := runtime.NewScheme()
	require.NoError(t, v1.AddToScheme(scheme))
	require.NoError(t, appsv1.AddToScheme(scheme))

	store := &v1.Store{
		TypeMeta:   metav1.TypeMeta{APIVersion: v1.GroupVersion.String(), Kind: "Store"},
		ObjectMeta: metav1.ObjectMeta{Name: "store", Namespace: "shop", UID: "store-uid"},
	}
	controllerRef := *metav1.NewControllerRef(store, v1.GroupVersion.WithKind("Store"))

	owned := finishedDeployment()
	owned.Name = "owned"
	owned.Namespace = "shop"
	owned.OwnerReferences = []metav1.OwnerReference{controllerRef}

	foreign := finishedDeployment()
	foreign.Name = "foreign"
	foreign.Namespace = "shop"
	foreign.Status.AvailableReplicas = 0

	t.Run("ignores deployments not controlled by the store", func(t *testing.T) {
		c := fake.NewClientBuilder().WithScheme(scheme).WithObjects(owned.DeepCopy(), foreign.DeepCopy()).Build()
		inProgress, err := RolloutInProgress(context.Background(), c, store)
		require.NoError(t, err)
		require.False(t, inProgress)
	})

	t.Run("detects owned deployment rolling out", func(t *testing.T) {
		rolling := owned.DeepCopy()
		rolling.Status.UpdatedReplicas = 1
		c := fake.NewClientBuilder().WithScheme(scheme).WithObjects(rolling, foreign.DeepCopy()).Build()
		inProgress, err := RolloutInProgress(context.Background(), c, store)
		require.NoError(t, err)
		require.True(t, inProgress)
	})
}
