package tracing

import (
	"context"
	"fmt"
	"os"
	"strings"
	"unicode/utf8"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracegrpc"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracehttp"
	"go.opentelemetry.io/otel/propagation"
	"go.opentelemetry.io/otel/sdk/resource"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	semconv "go.opentelemetry.io/otel/semconv/v1.26.0"
	"go.opentelemetry.io/otel/trace"
	ctrl "sigs.k8s.io/controller-runtime"
)

const (
	tracerName         = "github.com/shopware/shopware-operator"
	defaultServiceName = "shopware-operator"

	AttrNamespace     = attribute.Key("k8s.namespace.name")
	AttrName          = attribute.Key("shopware.resource.name")
	AttrKind          = attribute.Key("shopware.resource.kind")
	AttrState         = attribute.Key("shopware.state")
	AttrStateFrom     = attribute.Key("shopware.state.from")
	AttrStateTo       = attribute.Key("shopware.state.to")
	AttrMessage       = attribute.Key("shopware.status.message")
	AttrRequeueAfter  = attribute.Key("reconcile.requeue_after")
	EventStateChanged = "state changed"
	EventStatusUpdate = "status updated"
)

type Shutdown func(context.Context) error

func Setup(ctx context.Context, enabled bool, version string) (Shutdown, error) {
	if !enabled {
		return func(context.Context) error { return nil }, nil
	}

	exporter, err := newExporter(ctx)
	if err != nil {
		return nil, fmt.Errorf("create otlp trace exporter: %w", err)
	}

	res, err := resource.New(ctx,
		resource.WithTelemetrySDK(),
		resource.WithAttributes(
			semconv.ServiceName(defaultServiceName),
			semconv.ServiceVersion(version),
		),
		resource.WithFromEnv(),
	)
	if err != nil {
		return nil, fmt.Errorf("create otel resource: %w", err)
	}

	provider := sdktrace.NewTracerProvider(
		sdktrace.WithBatcher(exporter),
		sdktrace.WithResource(res),
	)
	otel.SetTracerProvider(provider)
	otel.SetTextMapPropagator(propagation.NewCompositeTextMapPropagator(
		propagation.TraceContext{},
		propagation.Baggage{},
	))

	return provider.Shutdown, nil
}

func newExporter(ctx context.Context) (sdktrace.SpanExporter, error) {
	protocol := os.Getenv("OTEL_EXPORTER_OTLP_TRACES_PROTOCOL")
	if protocol == "" {
		protocol = os.Getenv("OTEL_EXPORTER_OTLP_PROTOCOL")
	}

	switch strings.ToLower(protocol) {
	case "grpc":
		return otlptracegrpc.New(ctx)
	case "", "http/protobuf":
		return otlptracehttp.New(ctx)
	default:
		return nil, fmt.Errorf("unsupported otlp protocol %q, possible values: grpc, http/protobuf", protocol)
	}
}

func Tracer() trace.Tracer {
	return otel.Tracer(tracerName)
}

func Start(ctx context.Context, name string, attrs ...attribute.KeyValue) (context.Context, trace.Span) {
	return Tracer().Start(ctx, name, trace.WithAttributes(attrs...))
}

func StartReconcile(ctx context.Context, kind string, req ctrl.Request) (context.Context, trace.Span) {
	return Start(ctx, kind+".Reconcile",
		AttrKind.String(kind),
		AttrNamespace.String(req.Namespace),
		AttrName.String(req.Name),
	)
}

func End(span trace.Span, err *error) {
	if err != nil && *err != nil {
		RecordError(span, *err)
	}
	span.End()
}

func EndReconcile(span trace.Span, result ctrl.Result, err *error) {
	span.SetAttributes(AttrRequeueAfter.String(result.RequeueAfter.String()))
	End(span, err)
}

func RecordError(span trace.Span, err error) {
	message := validUTF8(err.Error())
	span.AddEvent(semconv.ExceptionEventName, trace.WithAttributes(
		semconv.ExceptionType(fmt.Sprintf("%T", err)),
		semconv.ExceptionMessage(message),
	))
	span.SetStatus(codes.Error, message)
}

func validUTF8(s string) string {
	return strings.ToValidUTF8(s, string(utf8.RuneError))
}

func RecordStateChange(ctx context.Context, from, to string) {
	span := trace.SpanFromContext(ctx)
	span.SetAttributes(AttrState.String(to))
	if from == to {
		return
	}
	span.AddEvent(EventStateChanged, trace.WithAttributes(
		AttrStateFrom.String(from),
		AttrStateTo.String(to),
	))
}

func RecordStatusUpdate(ctx context.Context, state, message string, attrs ...attribute.KeyValue) {
	trace.SpanFromContext(ctx).AddEvent(EventStatusUpdate, trace.WithAttributes(
		append([]attribute.KeyValue{
			AttrState.String(state),
			AttrMessage.String(validUTF8(message)),
		}, attrs...)...,
	))
}

func TraceID(ctx context.Context) string {
	spanCtx := trace.SpanContextFromContext(ctx)
	if !spanCtx.HasTraceID() {
		return ""
	}
	return spanCtx.TraceID().String()
}
