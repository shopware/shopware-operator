package k8s

import (
	"testing"

	"github.com/shopware/shopware-operator/internal/util"
	"github.com/stretchr/testify/assert"
	appsv1 "k8s.io/api/apps/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func describedDeployment() *appsv1.Deployment {
	return &appsv1.Deployment{ObjectMeta: metav1.ObjectMeta{
		Name:            "my-store-storefront",
		Namespace:       "shop",
		ResourceVersion: "1",
		Generation:      1,
		Labels:          map[string]string{"app": "storefront"},
		Annotations:     map[string]string{util.LastConfigHashAnnotation: "a"},
	}}
}

func TestDescribeUpdate(t *testing.T) {
	tests := []struct {
		name    string
		mutate  func(*appsv1.Deployment)
		reason  string
		changes []string
	}{
		{
			name:    "status only",
			mutate:  func(d *appsv1.Deployment) { d.Status.ReadyReplicas = 2 },
			reason:  "only status or resourceVersion changed",
			changes: []string{"resourceVersion:1->2"},
		},
		{
			name:    "generation",
			mutate:  func(d *appsv1.Deployment) { d.Generation = 2 },
			reason:  "generation changed",
			changes: []string{"generation:1->2"},
		},
		{
			name:    "config hash",
			mutate:  func(d *appsv1.Deployment) { d.Annotations[util.LastConfigHashAnnotation] = "b" },
			reason:  "config hash annotation changed",
			changes: []string{"hash:a->b"},
		},
		{
			name:    "label",
			mutate:  func(d *appsv1.Deployment) { d.Labels["extra"] = "x" },
			reason:  "labels or annotations changed",
			changes: []string{"label:extra"},
		},
		{
			name:    "ignored revision annotation",
			mutate:  func(d *appsv1.Deployment) { d.Annotations["deployment.kubernetes.io/revision"] = "3" },
			reason:  "only status or resourceVersion changed",
			changes: []string{"resourceVersion:1->2"},
		},
		{
			name: "deletion",
			mutate: func(d *appsv1.Deployment) {
				now := metav1.Now()
				d.DeletionTimestamp = &now
			},
			reason: "deletion requested",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			oldObject := describedDeployment()
			newObject := oldObject.DeepCopy()
			newObject.ResourceVersion = "2"
			tt.mutate(newObject)

			reason, changes := DescribeUpdate(oldObject, newObject)
			assert.Equal(t, tt.reason, reason)
			assert.Equal(t, tt.changes, changes)
			assert.Equal(t, describedDeployment().Annotations, oldObject.Annotations)
		})
	}
}
