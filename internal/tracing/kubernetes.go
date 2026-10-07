package tracing

import (
	"net/http"

	"go.opentelemetry.io/contrib/instrumentation/net/http/otelhttp"
	"go.opentelemetry.io/otel/trace"
	"k8s.io/client-go/rest"
)

func WrapKubernetesClient(cfg *rest.Config) {
	cfg.Wrap(func(rt http.RoundTripper) http.RoundTripper {
		return otelhttp.NewTransport(rt,
			otelhttp.WithFilter(hasParentSpan),
			otelhttp.WithSpanNameFormatter(kubernetesSpanName),
		)
	})
}

func hasParentSpan(r *http.Request) bool {
	return trace.SpanContextFromContext(r.Context()).IsValid()
}

func kubernetesSpanName(_ string, r *http.Request) string {
	return "k8s " + r.Method + " " + r.URL.Path
}
