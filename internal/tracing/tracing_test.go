package tracing

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/codes"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/sdk/trace/tracetest"
	semconv "go.opentelemetry.io/otel/semconv/v1.26.0"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
)

func newRecorder(t *testing.T) *tracetest.SpanRecorder {
	t.Helper()
	recorder := tracetest.NewSpanRecorder()
	previous := otel.GetTracerProvider()
	otel.SetTracerProvider(sdktrace.NewTracerProvider(sdktrace.WithSpanProcessor(recorder)))
	t.Cleanup(func() { otel.SetTracerProvider(previous) })
	return recorder
}

func TestReconcileSpanRecordsStateChangeAndStatus(t *testing.T) {
	recorder := newRecorder(t)
	req := ctrl.Request{NamespacedName: types.NamespacedName{Namespace: "ns", Name: "shop"}}

	ctx, span := StartReconcile(context.Background(), "Store", req)
	RecordStateChange(ctx, "wait", "setup")
	RecordStatusUpdate(ctx, "setup", "running setup")
	var err error
	EndReconcile(span, ctrl.Result{RequeueAfter: 5 * time.Second}, &err)

	spans := recorder.Ended()
	require.Len(t, spans, 1)
	s := spans[0]
	assert.Equal(t, "Store.Reconcile", s.Name())
	assert.Equal(t, codes.Unset, s.Status().Code)
	assert.Contains(t, s.Attributes(), AttrNamespace.String("ns"))
	assert.Contains(t, s.Attributes(), AttrName.String("shop"))
	assert.Contains(t, s.Attributes(), AttrState.String("setup"))
	assert.Contains(t, s.Attributes(), AttrRequeueAfter.String("5s"))

	require.Len(t, s.Events(), 2)
	assert.Equal(t, EventStateChanged, s.Events()[0].Name)
	assert.Contains(t, s.Events()[0].Attributes, AttrStateFrom.String("wait"))
	assert.Contains(t, s.Events()[0].Attributes, AttrStateTo.String("setup"))
	assert.Equal(t, EventStatusUpdate, s.Events()[1].Name)
	assert.Contains(t, s.Events()[1].Attributes, AttrMessage.String("running setup"))
}

func TestStateChangeWithoutTransitionAddsNoEvent(t *testing.T) {
	recorder := newRecorder(t)

	ctx, span := Start(context.Background(), "step")
	RecordStateChange(ctx, "ready", "ready")
	span.End()

	spans := recorder.Ended()
	require.Len(t, spans, 1)
	assert.Empty(t, spans[0].Events())
}

func TestEndRecordsError(t *testing.T) {
	recorder := newRecorder(t)

	_, span := Start(context.Background(), "step")
	err := errors.New("boom")
	End(span, &err)

	spans := recorder.Ended()
	require.Len(t, spans, 1)
	assert.Equal(t, codes.Error, spans[0].Status().Code)
	assert.Equal(t, "boom", spans[0].Status().Description)
}

func TestRecordErrorSanitizesInvalidUTF8(t *testing.T) {
	recorder := newRecorder(t)

	_, span := Start(context.Background(), "step")
	err := errors.New("stderr: \xff broken")
	End(span, &err)

	spans := recorder.Ended()
	require.Len(t, spans, 1)
	assert.Equal(t, codes.Error, spans[0].Status().Code)
	assert.Equal(t, "stderr: \uFFFD broken", spans[0].Status().Description)

	require.Len(t, spans[0].Events(), 1)
	ev := spans[0].Events()[0]
	assert.Equal(t, semconv.ExceptionEventName, ev.Name)
	assert.Contains(t, ev.Attributes, semconv.ExceptionMessage("stderr: \uFFFD broken"))
	assert.Contains(t, ev.Attributes, semconv.ExceptionType("*errors.errorString"))
}

func TestSetupDisabledIsNoop(t *testing.T) {
	shutdown, err := Setup(context.Background(), false, "dev")
	require.NoError(t, err)
	assert.NoError(t, shutdown(context.Background()))
}

func TestTraceIDFromContext(t *testing.T) {
	newRecorder(t)

	assert.Empty(t, TraceID(context.Background()))

	ctx, span := Start(context.Background(), "step")
	defer span.End()

	assert.Equal(t, span.SpanContext().TraceID().String(), TraceID(ctx))
}
