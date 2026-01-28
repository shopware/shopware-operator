package controller

import (
	"context"
	"testing"

	v1 "github.com/shopware/shopware-operator/api/v1"
	"github.com/shopware/shopware-operator/internal/job"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

func failedSnapshotPod(phase corev1.PodPhase) *corev1.Pod {
	return &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "snapshot-pod",
			Namespace: "shop",
			Labels:    map[string]string{"job-name": "snapshot"},
		},
		Status: corev1.PodStatus{
			Phase: phase,
			ContainerStatuses: []corev1.ContainerStatus{{
				Name:  job.CONTAINER_NAME_SNAPSHOT,
				State: corev1.ContainerState{Terminated: &corev1.ContainerStateTerminated{ExitCode: 1}},
			}},
		},
	}
}

func snapshotJob(conditions ...batchv1.JobCondition) *batchv1.Job {
	return &batchv1.Job{
		ObjectMeta: metav1.ObjectMeta{Name: "snapshot", Namespace: "shop"},
		Spec: batchv1.JobSpec{
			Selector: &metav1.LabelSelector{MatchLabels: map[string]string{"job-name": "snapshot"}},
			Template: corev1.PodTemplateSpec{Spec: corev1.PodSpec{
				Containers: []corev1.Container{{Name: job.CONTAINER_NAME_SNAPSHOT}},
			}},
		},
		Status: batchv1.JobStatus{Failed: 1, Conditions: conditions},
	}
}

func snapshotStateFor(t *testing.T, snapshotJob *batchv1.Job, pod *corev1.Pod) v1.SnapshotState {
	t.Helper()

	scheme := runtime.NewScheme()
	require.NoError(t, v1.AddToScheme(scheme))
	require.NoError(t, corev1.AddToScheme(scheme))
	require.NoError(t, batchv1.AddToScheme(scheme))

	r := &StoreSnapshotBaseReconciler{
		Client: fake.NewClientBuilder().WithScheme(scheme).WithObjects(snapshotJob, pod).Build(),
		Scheme: scheme,
		Logger: zap.NewNop().Sugar(),
	}
	snapshot := &v1.StoreSnapshotCreate{
		ObjectMeta: metav1.ObjectMeta{Name: "snapshot", Namespace: "shop"},
		Status:     v1.StoreSnapshotStatus{State: v1.SnapshotStateRunning},
	}
	getJob := func(context.Context, client.Client, v1.Store, SnapshotResource) (*batchv1.Job, error) {
		return snapshotJob, nil
	}

	require.NoError(t, r.reconcileCRStatus(context.Background(), v1.Store{}, snapshot, getJob))
	return snapshot.Status.State
}

func TestSnapshotStatusWaitsForJobRetries(t *testing.T) {
	require.Equal(t, v1.SnapshotStateRunning, snapshotStateFor(t, snapshotJob(), failedSnapshotPod(corev1.PodFailed)))
}

func TestSnapshotStatusFailsOnTerminalJobFailure(t *testing.T) {
	failed := snapshotJob(batchv1.JobCondition{Type: batchv1.JobFailed, Status: corev1.ConditionTrue})
	require.Equal(t, v1.SnapshotStateFailed, snapshotStateFor(t, failed, failedSnapshotPod(corev1.PodFailed)))
}

func TestSnapshotStatusFailsWhenSidecarKeepsPodRunning(t *testing.T) {
	require.Equal(t, v1.SnapshotStateFailed, snapshotStateFor(t, snapshotJob(), failedSnapshotPod(corev1.PodRunning)))
}
