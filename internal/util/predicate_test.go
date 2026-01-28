package util_test

import (
	"testing"

	"github.com/shopware/shopware-operator/internal/util"
	"github.com/stretchr/testify/require"
	appsv1 "k8s.io/api/apps/v1"
	batchv1 "k8s.io/api/batch/v1"
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

func TestJobLifecyclePredicate(t *testing.T) {
	p := util.JobLifecyclePredicate()

	running := &batchv1.Job{
		ObjectMeta: metav1.ObjectMeta{
			Name:            "snapshot",
			ResourceVersion: "1",
			Annotations:     map[string]string{util.LastConfigHashAnnotation: "a"},
		},
		Status: batchv1.JobStatus{Active: 1},
	}

	tests := []struct {
		name   string
		modify func(job *batchv1.Job)
		want   bool
	}{
		{
			name: "only resource version changed",
			modify: func(job *batchv1.Job) {
				job.ResourceVersion = "2"
			},
			want: false,
		},
		{
			name: "hash annotation changed",
			modify: func(job *batchv1.Job) {
				job.Annotations[util.LastConfigHashAnnotation] = "b"
			},
			want: true,
		},
		{
			name: "job succeeded",
			modify: func(job *batchv1.Job) {
				job.Status.Active = 0
				job.Status.Succeeded = 1
			},
			want: true,
		},
		{
			name: "job failed",
			modify: func(job *batchv1.Job) {
				job.Status.Failed = 1
			},
			want: true,
		},
		{
			name: "complete condition added",
			modify: func(job *batchv1.Job) {
				job.Status.Conditions = []batchv1.JobCondition{{
					Type:   batchv1.JobComplete,
					Status: corev1.ConditionTrue,
				}}
			},
			want: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			updated := running.DeepCopy()
			tt.modify(updated)
			require.Equal(t, tt.want, p.Update(event.UpdateEvent{ObjectOld: running, ObjectNew: updated}))
		})
	}

	require.True(t, p.Create(event.CreateEvent{Object: running}))
	require.True(t, p.Delete(event.DeleteEvent{Object: running}))
	require.False(t, p.Generic(event.GenericEvent{Object: running}))
}

func TestMetadataChangedPredicate(t *testing.T) {
	p := util.MetadataChangedPredicate()

	base := &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:            "test",
			ResourceVersion: "1",
			Labels:          map[string]string{"app": "shop"},
			Annotations: map[string]string{
				util.LastConfigHashAnnotation:       "a",
				"deployment.kubernetes.io/revision": "1",
				"custom":                            "value",
			},
		},
	}

	tests := []struct {
		name   string
		modify func(d *appsv1.Deployment)
		want   bool
	}{
		{
			name: "only resource version changed",
			modify: func(d *appsv1.Deployment) {
				d.ResourceVersion = "2"
			},
			want: false,
		},
		{
			name: "status changed",
			modify: func(d *appsv1.Deployment) {
				d.Status.AvailableReplicas = 3
			},
			want: false,
		},
		{
			name: "controller managed annotation changed",
			modify: func(d *appsv1.Deployment) {
				d.Annotations["deployment.kubernetes.io/revision"] = "2"
			},
			want: false,
		},
		{
			name: "managed label removed",
			modify: func(d *appsv1.Deployment) {
				delete(d.Labels, "app")
			},
			want: true,
		},
		{
			name: "label added",
			modify: func(d *appsv1.Deployment) {
				d.Labels["team"] = "x"
			},
			want: true,
		},
		{
			name: "annotation changed",
			modify: func(d *appsv1.Deployment) {
				d.Annotations["custom"] = "other"
			},
			want: true,
		},
		{
			name: "annotation removed",
			modify: func(d *appsv1.Deployment) {
				delete(d.Annotations, "custom")
			},
			want: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			updated := base.DeepCopy()
			tt.modify(updated)
			require.Equal(t, tt.want, p.Update(event.UpdateEvent{ObjectOld: base, ObjectNew: updated}))
		})
	}

	require.False(t, p.Create(event.CreateEvent{Object: base}))
	require.False(t, p.Delete(event.DeleteEvent{Object: base}))
	require.False(t, p.Generic(event.GenericEvent{Object: base}))
}

func TestMetadataChangedPredicateDetectsRenamedEmptyValues(t *testing.T) {
	p := util.MetadataChangedPredicate()

	old := &appsv1.Deployment{ObjectMeta: metav1.ObjectMeta{
		Labels:      map[string]string{"old-marker": ""},
		Annotations: map[string]string{"old-note": ""},
	}}
	renamedLabel := old.DeepCopy()
	renamedLabel.Labels = map[string]string{"new-marker": ""}
	renamedAnnotation := old.DeepCopy()
	renamedAnnotation.Annotations = map[string]string{"new-note": ""}

	require.True(t, p.Update(event.UpdateEvent{ObjectOld: old, ObjectNew: renamedLabel}))
	require.True(t, p.Update(event.UpdateEvent{ObjectOld: old, ObjectNew: renamedAnnotation}))
}

func TestDeletingPredicate(t *testing.T) {
	p := util.DeletingPredicate()

	alive := &appsv1.Deployment{ObjectMeta: metav1.ObjectMeta{Name: "test", ResourceVersion: "1"}}
	deleting := alive.DeepCopy()
	deleting.ResourceVersion = "2"
	now := metav1.Now()
	deleting.DeletionTimestamp = &now

	require.False(t, p.Update(event.UpdateEvent{ObjectOld: alive, ObjectNew: alive}))
	require.True(t, p.Update(event.UpdateEvent{ObjectOld: alive, ObjectNew: deleting}))
	require.True(t, p.Delete(event.DeleteEvent{Object: alive}))
	require.False(t, p.Create(event.CreateEvent{Object: alive}))
	require.False(t, p.Generic(event.GenericEvent{Object: alive}))
}
