package k8s

import (
	"context"
	"testing"

	"github.com/shopware/shopware-operator/internal/util"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	appsv1 "k8s.io/api/apps/v1"
	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

func desiredDeployment(secretHash string) *appsv1.Deployment {
	annotations := map[string]string{"custom": "value"}
	dep := &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:        "my-shop-storefront",
			Namespace:   "shopware",
			Annotations: annotations,
		},
		Spec: appsv1.DeploymentSpec{
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{Annotations: annotations},
			},
		},
	}
	SetSecretHash(dep, secretHash)
	return dep
}

func deployedDeployment(t *testing.T, secretHash string) *appsv1.Deployment {
	t.Helper()
	dep := desiredDeployment(secretHash)
	hash, err := ObjectHash(dep)
	require.NoError(t, err)
	dep.Annotations = map[string]string{"custom": "value", util.LastConfigHashAnnotation: hash}
	return dep
}

func TestSetSecretHash(t *testing.T) {
	dep := desiredDeployment("abc")
	assert.Equal(t, "abc", dep.Spec.Template.Annotations[util.StoreSecretHashAnnotation])
	assert.NotContains(t, dep.Annotations, util.StoreSecretHashAnnotation)

	job := &batchv1.Job{}
	SetSecretHash(job, "abc")
	assert.Equal(t, "abc", job.Spec.Template.Annotations[util.StoreSecretHashAnnotation])

	cronJob := &batchv1.CronJob{}
	SetSecretHash(cronJob, "abc")
	assert.Equal(t, "abc", cronJob.Spec.JobTemplate.Spec.Template.Annotations[util.StoreSecretHashAnnotation])

	empty := desiredDeployment("")
	assert.NotContains(t, empty.Spec.Template.Annotations, util.StoreSecretHashAnnotation)
}

func TestSecretHashIsNotPartOfConfigHash(t *testing.T) {
	first, err := ObjectHash(desiredDeployment("abc"))
	require.NoError(t, err)
	second, err := ObjectHash(desiredDeployment("def"))
	require.NoError(t, err)
	assert.Equal(t, first, second)
}

func TestDetectObjectChangeSecretHash(t *testing.T) {
	tests := []struct {
		name     string
		deployed string
		desired  string
		reason   string
	}{
		{name: "secret changed", deployed: "abc", desired: "def", reason: changeReasonSecretChanged},
		{name: "secret hash missing on deployed object", deployed: "", desired: "def", reason: changeReasonSecretChanged},
		{name: "secret unchanged", deployed: "abc", desired: "abc"},
		{name: "no secret hash desired", deployed: "abc", desired: ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cl := fake.NewClientBuilder().WithObjects(deployedDeployment(t, tt.deployed)).Build()
			change, err := DetectObjectChange(context.Background(), cl, desiredDeployment(tt.desired))
			require.NoError(t, err)
			assert.Equal(t, tt.reason, change.Reason)
		})
	}
}

func TestEnsureObjectWithHashUpdatesOnSecretChange(t *testing.T) {
	scheme := runtime.NewScheme()
	require.NoError(t, appsv1.AddToScheme(scheme))
	cl := fake.NewClientBuilder().WithScheme(scheme).WithObjects(deployedDeployment(t, "abc")).Build()

	require.NoError(t, EnsureObjectWithHash(context.Background(), cl, nil, desiredDeployment("def"), scheme))

	live := &appsv1.Deployment{}
	require.NoError(t, cl.Get(context.Background(), types.NamespacedName{Namespace: "shopware", Name: "my-shop-storefront"}, live))
	assert.Equal(t, "def", live.Spec.Template.Annotations[util.StoreSecretHashAnnotation])
}
