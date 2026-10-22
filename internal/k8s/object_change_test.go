package k8s_test

import (
	"context"
	"testing"

	"github.com/shopware/shopware-operator/internal/k8s"
	"github.com/shopware/shopware-operator/internal/tracing"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/otel"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/sdk/trace/tracetest"
	policy "k8s.io/api/policy/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

func desiredPDB() *policy.PodDisruptionBudget {
	minAvailable := intstr.FromInt32(1)
	return &policy.PodDisruptionBudget{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "my-shop-admin",
			Namespace: "shopware",
			Labels:    map[string]string{"app": "admin"},
		},
		Spec: policy.PodDisruptionBudgetSpec{MinAvailable: &minAvailable},
	}
}

func deployedPDB(t *testing.T, mutate func(*policy.PodDisruptionBudget)) *policy.PodDisruptionBudget {
	t.Helper()
	obj := desiredPDB()
	hash, err := k8s.ObjectHash(obj)
	require.NoError(t, err)
	obj.Annotations = map[string]string{"shopware.com/last-config-hash": hash}
	if mutate != nil {
		mutate(obj)
	}
	return obj
}

func TestDetectObjectChange(t *testing.T) {
	tests := []struct {
		name     string
		existing client.Object
		reason   string
		keys     []string
	}{
		{name: "not found", reason: k8s.ChangeReasonNotFound},
		{
			name: "no hash annotation",
			existing: deployedPDB(t, func(p *policy.PodDisruptionBudget) {
				p.Annotations = nil
			}),
			reason: k8s.ChangeReasonHashMissing,
		},
		{
			name: "hash changed",
			existing: deployedPDB(t, func(p *policy.PodDisruptionBudget) {
				p.Annotations["shopware.com/last-config-hash"] = "outdated"
			}),
			reason: k8s.ChangeReasonHashChanged,
		},
		{
			name: "label changed",
			existing: deployedPDB(t, func(p *policy.PodDisruptionBudget) {
				p.Labels["app"] = "other"
				p.Labels["extra"] = "x"
			}),
			reason: k8s.ChangeReasonMetadata,
			keys:   []string{"label:app", "label:extra"},
		},
		{name: "in place", existing: deployedPDB(t, nil)},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			builder := fake.NewClientBuilder()
			if tt.existing != nil {
				builder = builder.WithObjects(tt.existing)
			}

			change, err := k8s.DetectObjectChange(context.Background(), builder.Build(), desiredPDB())
			require.NoError(t, err)
			assert.Equal(t, tt.reason, change.Reason)
			assert.Equal(t, tt.reason != "", change.Changed())
			assert.Equal(t, tt.keys, change.ChangedKeys)
		})
	}
}

func TestHasObjectChangedRecordsSpanEvent(t *testing.T) {
	recorder := tracetest.NewSpanRecorder()
	previous := otel.GetTracerProvider()
	otel.SetTracerProvider(sdktrace.NewTracerProvider(sdktrace.WithSpanProcessor(recorder)))
	t.Cleanup(func() { otel.SetTracerProvider(previous) })

	cl := fake.NewClientBuilder().WithObjects(deployedPDB(t, nil)).Build()

	ctx, span := tracing.Start(context.Background(), "Base.ReconcilePDB")
	changed, err := k8s.HasObjectChanged(ctx, cl, desiredPDB())
	require.NoError(t, err)
	assert.False(t, changed)

	missing := desiredPDB()
	missing.Name = "my-shop-worker"
	changed, err = k8s.HasObjectChanged(ctx, cl, missing)
	require.NoError(t, err)
	assert.True(t, changed)
	span.End()

	spans := recorder.Ended()
	require.Len(t, spans, 1)
	events := spans[0].Events()
	require.Len(t, events, 2)

	assert.Equal(t, tracing.EventObjectInPlace, events[0].Name)
	assert.Contains(t, events[0].Attributes, tracing.AttrObjectKind.String("PodDisruptionBudget"))
	assert.Contains(t, events[0].Attributes, tracing.AttrObjectMessage.String("PodDisruptionBudget my-shop-admin is in place, nothing changed"))

	assert.Equal(t, tracing.EventObjectChanged, events[1].Name)
	assert.Contains(t, events[1].Attributes, tracing.AttrObjectName.String("my-shop-worker"))
	assert.Contains(t, events[1].Attributes, tracing.AttrObjectReason.String(k8s.ChangeReasonNotFound))
}
