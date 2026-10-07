package tracing

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"k8s.io/client-go/rest"
)

func newKubernetesClient(t *testing.T) (*http.Client, string) {
	t.Helper()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	t.Cleanup(server.Close)

	cfg := &rest.Config{Host: server.URL}
	WrapKubernetesClient(cfg)
	httpClient, err := rest.HTTPClientFor(cfg)
	require.NoError(t, err)
	return httpClient, server.URL
}

func doRequest(t *testing.T, ctx context.Context, httpClient *http.Client, url string) {
	t.Helper()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	require.NoError(t, err)
	resp, err := httpClient.Do(req)
	require.NoError(t, err)
	require.NoError(t, resp.Body.Close())
}

func TestKubernetesClientTracesRequestsWithinReconcile(t *testing.T) {
	recorder := newRecorder(t)
	httpClient, url := newKubernetesClient(t)

	ctx, parent := Start(context.Background(), "Store.Reconcile")
	doRequest(t, ctx, httpClient, url+"/apis/shop.shopware.com/v1/namespaces/ns/stores/shop")
	parent.End()

	spans := recorder.Ended()
	require.Len(t, spans, 2)
	assert.Equal(t, "k8s GET /apis/shop.shopware.com/v1/namespaces/ns/stores/shop", spans[0].Name())
	assert.Equal(t, parent.SpanContext().SpanID(), spans[0].Parent().SpanID())
}

func TestKubernetesClientSkipsRequestsWithoutParentSpan(t *testing.T) {
	recorder := newRecorder(t)
	httpClient, url := newKubernetesClient(t)

	doRequest(t, context.Background(), httpClient, url+"/api/v1/namespaces/ns/secrets")

	assert.Empty(t, recorder.Ended())
}
