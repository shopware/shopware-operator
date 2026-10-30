package util_test

import (
	"testing"

	"github.com/shopware/shopware-operator/internal/util"
	"github.com/stretchr/testify/require"
	appsv1 "k8s.io/api/apps/v1"
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
