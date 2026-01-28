package util_test

import (
	"testing"
	"time"

	"github.com/shopware/shopware-operator/internal/util"
	"github.com/stretchr/testify/require"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/event"
)

func deploymentWithHash(hash, resourceVersion string) *appsv1.Deployment {
	return &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:            "test",
			ResourceVersion: resourceVersion,
			Annotations:     map[string]string{util.LastConfigHashAnnotation: hash},
		},
	}
}

func TestHashAnnotationChangedPredicate(t *testing.T) {
	p := util.HashAnnotationChangedPredicate()

	tests := []struct {
		name string
		old  *appsv1.Deployment
		new  *appsv1.Deployment
		want bool
	}{
		{
			name: "same hash with new resource version",
			old:  deploymentWithHash("a", "1"),
			new:  deploymentWithHash("a", "2"),
			want: false,
		},
		{
			name: "changed hash",
			old:  deploymentWithHash("a", "1"),
			new:  deploymentWithHash("b", "2"),
			want: true,
		},
		{
			name: "hash removed",
			old:  deploymentWithHash("a", "1"),
			new:  &appsv1.Deployment{ObjectMeta: metav1.ObjectMeta{Name: "test", ResourceVersion: "2"}},
			want: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			require.Equal(t, tt.want, p.Update(event.UpdateEvent{ObjectOld: tt.old, ObjectNew: tt.new}))
		})
	}

	require.False(t, p.Create(event.CreateEvent{Object: deploymentWithHash("a", "1")}))
	require.True(t, p.Delete(event.DeleteEvent{Object: deploymentWithHash("a", "1")}))
	require.False(t, p.Generic(event.GenericEvent{Object: deploymentWithHash("a", "1")}))
}

func deploymentWithStatus(status appsv1.DeploymentStatus) *appsv1.Deployment {
	return &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{Name: "test"},
		Status:     status,
	}
}

func TestDeploymentStatusChangedPredicate(t *testing.T) {
	p := util.DeploymentStatusChangedPredicate()

	base := appsv1.DeploymentStatus{
		Replicas:          2,
		AvailableReplicas: 1,
		Conditions: []appsv1.DeploymentCondition{
			{
				Type:           appsv1.DeploymentProgressing,
				Status:         corev1.ConditionTrue,
				Reason:         "ReplicaSetUpdated",
				LastUpdateTime: metav1.NewTime(time.Unix(1, 0)),
			},
		},
	}

	tests := []struct {
		name   string
		modify func(status *appsv1.DeploymentStatus)
		want   bool
	}{
		{
			name:   "nothing changed",
			modify: func(*appsv1.DeploymentStatus) {},
			want:   false,
		},
		{
			name: "only condition timestamp changed",
			modify: func(status *appsv1.DeploymentStatus) {
				status.Conditions[0].LastUpdateTime = metav1.NewTime(time.Unix(2, 0))
			},
			want: false,
		},
		{
			name: "unused field changed",
			modify: func(status *appsv1.DeploymentStatus) {
				status.ObservedGeneration = 5
				status.UpdatedReplicas = 2
			},
			want: false,
		},
		{
			name: "available replicas changed",
			modify: func(status *appsv1.DeploymentStatus) {
				status.AvailableReplicas = 2
			},
			want: true,
		},
		{
			name: "unavailable replicas changed",
			modify: func(status *appsv1.DeploymentStatus) {
				status.UnavailableReplicas = 1
			},
			want: true,
		},
		{
			name: "progress deadline exceeded",
			modify: func(status *appsv1.DeploymentStatus) {
				status.Conditions[0].Status = corev1.ConditionFalse
				status.Conditions[0].Reason = "ProgressDeadlineExceeded"
			},
			want: true,
		},
		{
			name: "condition added",
			modify: func(status *appsv1.DeploymentStatus) {
				status.Conditions = append(status.Conditions, appsv1.DeploymentCondition{Type: appsv1.DeploymentAvailable})
			},
			want: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			newStatus := *base.DeepCopy()
			tt.modify(&newStatus)
			require.Equal(t, tt.want, p.Update(event.UpdateEvent{
				ObjectOld: deploymentWithStatus(*base.DeepCopy()),
				ObjectNew: deploymentWithStatus(newStatus),
			}))
		})
	}

	require.False(t, p.Create(event.CreateEvent{Object: deploymentWithStatus(base)}))
	require.False(t, p.Delete(event.DeleteEvent{Object: deploymentWithStatus(base)}))
}
