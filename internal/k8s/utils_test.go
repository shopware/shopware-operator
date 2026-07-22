package k8s

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	appsv1 "k8s.io/api/apps/v1"
	autoscalingv2 "k8s.io/api/autoscaling/v2"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/utils/ptr"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

func TestEnsureDeploymentPreservesAutoscaledReplicas(t *testing.T) {
	ctx := context.Background()
	scheme := newScheme(t)

	existing := testDeployment(6, "shopware:v1")
	setDeploymentHash(t, existing, true)
	hpa := &autoscalingv2.HorizontalPodAutoscaler{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "keda-hpa-store-storefront",
			Namespace: "shop",
		},
		Spec: autoscalingv2.HorizontalPodAutoscalerSpec{
			ScaleTargetRef: autoscalingv2.CrossVersionObjectReference{
				APIVersion: "apps/v1",
				Kind:       "Deployment",
				Name:       "store-storefront",
			},
		},
	}
	client := fake.NewClientBuilder().WithScheme(scheme).WithObjects(existing, hpa).Build()
	desired := testDeployment(2, "shopware:v2")

	changed, err := HasDeploymentChanged(ctx, client, desired)
	require.NoError(t, err)
	assert.True(t, changed)

	require.NoError(t, EnsureDeployment(ctx, client, nil, desired, scheme, true))

	updated := &appsv1.Deployment{}
	require.NoError(t, client.Get(ctx, objectKey(existing), updated))
	assert.Equal(t, int32(6), *updated.Spec.Replicas)
	assert.Equal(t, "shopware:v2", updated.Spec.Template.Spec.Containers[0].Image)

	updated.Spec.Replicas = ptr.To(int32(5))
	require.NoError(t, client.Update(ctx, updated))

	changed, err = HasDeploymentChanged(ctx, client, testDeployment(2, "shopware:v2"))
	require.NoError(t, err)
	assert.False(t, changed)
}

func TestEnsureDeploymentAppliesConfiguredReplicasWithoutAutoscaler(t *testing.T) {
	ctx := context.Background()
	scheme := newScheme(t)

	existing := testDeployment(6, "shopware:v1")
	setDeploymentHash(t, existing, false)
	client := fake.NewClientBuilder().WithScheme(scheme).WithObjects(existing).Build()
	desired := testDeployment(2, "shopware:v2")

	changed, err := HasDeploymentChanged(ctx, client, desired)
	require.NoError(t, err)
	assert.True(t, changed)

	require.NoError(t, EnsureDeployment(ctx, client, nil, desired, scheme, true))

	updated := &appsv1.Deployment{}
	require.NoError(t, client.Get(ctx, objectKey(existing), updated))
	assert.Equal(t, int32(2), *updated.Spec.Replicas)
	assert.Equal(t, "shopware:v2", updated.Spec.Template.Spec.Containers[0].Image)
}

func newScheme(t *testing.T) *runtime.Scheme {
	t.Helper()

	scheme := runtime.NewScheme()
	require.NoError(t, appsv1.AddToScheme(scheme))
	require.NoError(t, autoscalingv2.AddToScheme(scheme))
	return scheme
}

func testDeployment(replicas int32, image string) *appsv1.Deployment {
	const name = "store-storefront"

	return &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: "shop",
		},
		Spec: appsv1.DeploymentSpec{
			Replicas: ptr.To(replicas),
			Selector: &metav1.LabelSelector{
				MatchLabels: map[string]string{"app": name},
			},
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{
					Labels: map[string]string{"app": name},
				},
				Spec: corev1.PodSpec{
					Containers: []corev1.Container{{
						Name:  "shopware",
						Image: image,
					}},
				},
			},
		},
	}
}

func setDeploymentHash(t *testing.T, deployment *appsv1.Deployment, autoscaled bool) {
	t.Helper()

	hashObject := runtime.Object(deployment)
	if autoscaled {
		hashObject = deploymentHashObject(deployment)
	}
	hash, err := ObjectHash(hashObject)
	require.NoError(t, err)
	deployment.Annotations = map[string]string{"shopware.com/last-config-hash": hash}
}

func objectKey(object client.Object) client.ObjectKey {
	return client.ObjectKeyFromObject(object)
}
