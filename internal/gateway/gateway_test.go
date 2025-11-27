package gateway_test

import (
	"testing"

	v1 "github.com/shopware/shopware-operator/api/v1"
	"github.com/shopware/shopware-operator/internal/gateway"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func TestGetStoreRouteName(t *testing.T) {
	tests := []struct {
		name     string
		store    v1.Store
		host     string
		expected string
	}{
		{
			name: "simple host",
			store: v1.Store{
				ObjectMeta: metav1.ObjectMeta{
					Name: "shop1",
				},
			},
			host:     "example.com",
			expected: "store-shop1-example-com-route",
		},
		{
			name: "host with uppercase and symbols",
			store: v1.Store{
				ObjectMeta: metav1.ObjectMeta{
					Name: "shop2",
				},
			},
			host:     "My-Host_123.com",
			expected: "store-shop2-my-host-123-com-route",
		},
		{
			name: "host with spaces",
			store: v1.Store{
				ObjectMeta: metav1.ObjectMeta{
					Name: "shop3",
				},
			},
			host:     "foo bar.com",
			expected: "store-shop3-foo-bar-com-route",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := gateway.GetStoreRouteName(tt.store, tt.host)
			if result != tt.expected {
				t.Fatalf("got %s != %s expected", result, tt.expected)
			}
		})
	}
}

func TestStoreGateway(t *testing.T) {
	store := v1.Store{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "shop1",
			Namespace: "default",
		},
		Spec: v1.StoreSpec{
			Network: v1.NetworkSpec{
				Hosts:            []string{"example.com"},
				Host:             "fallback.com",
				TLSSecretName:    "tls-secret",
				IngressClassName: "nginx",
				Annotations:      map[string]string{"foo": "bar"},
				Labels:           map[string]string{"custom": "label"},
				Port:             8080,
			},
		},
	}

	gw := gateway.StoreGateway(store)

	// Name & namespace
	expectedName := gateway.GetStoreGatewayName(store)
	if gw.Name != expectedName {
		t.Errorf("expected name %q, got %q", expectedName, gw.Name)
	}
	if gw.Namespace != store.Namespace {
		t.Errorf("expected namespace %q, got %q", store.Namespace, gw.Namespace)
	}

	// Labels & annotations
	if gw.Labels["custom"] != "label" {
		t.Errorf("expected label 'custom: label', got %v", gw.Labels)
	}
	if gw.Annotations["foo"] != "bar" {
		t.Errorf("expected annotation 'foo: bar', got %v", gw.Annotations)
	}

	// GatewayClass
	if string(gw.Spec.GatewayClassName) != store.Spec.Network.IngressClassName {
		t.Errorf("expected gatewayClass %q, got %q", store.Spec.Network.IngressClassName, gw.Spec.GatewayClassName)
	}

	// Listeners: expect HTTP + HTTPS
	if len(gw.Spec.Listeners) != 2 {
		t.Errorf("expected 2 listeners, got %d", len(gw.Spec.Listeners))
	}

	httpListener := gw.Spec.Listeners[0]
	if httpListener.Port != 80 || httpListener.Protocol != "HTTP" {
		t.Errorf("expected HTTP listener port 80, got %d protocol %v", httpListener.Port, httpListener.Protocol)
	}

	httpsListener := gw.Spec.Listeners[1]
	if httpsListener.Port != 443 || httpsListener.Protocol != "HTTPS" {
		t.Errorf("expected HTTPS listener port 443, got %d protocol %v", httpsListener.Port, httpsListener.Protocol)
	}
	if httpsListener.TLS == nil || len(httpsListener.TLS.CertificateRefs) != 1 {
		t.Errorf("expected HTTPS TLS cert ref, got %+v", httpsListener.TLS)
	} else if string(httpsListener.TLS.CertificateRefs[0].Name) != store.Spec.Network.TLSSecretName {
		t.Errorf("expected TLS cert %q, got %q", store.Spec.Network.TLSSecretName, httpsListener.TLS.CertificateRefs[0].Name)
	}
}

func TestStoreHTTPRoutes(t *testing.T) {
	store := v1.Store{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "shop1",
			Namespace: "default",
		},
		Spec: v1.StoreSpec{
			Network: v1.NetworkSpec{
				Hosts:            []string{"example.com"},
				Host:             "fallback.com",
				TLSSecretName:    "tls-secret",
				IngressClassName: "nginx",
				Annotations:      map[string]string{"foo": "bar"},
				Labels:           map[string]string{"custom": "label"},
				Port:             8080,
			},
		},
	}

	routes := gateway.StoreHTTPRoutes(store)

	// Expect 2 routes (Hosts + Host)
	if len(routes) != 2 {
		t.Errorf("expected 2 routes, got %d", len(routes))
	}

	for _, r := range routes {
		// Route name contains store name and host
		if r.Namespace != store.Namespace {
			t.Errorf("expected namespace %q, got %q", store.Namespace, r.Namespace)
		}
		if len(r.Spec.Hostnames) != 1 {
			t.Errorf("expected 1 hostname, got %d", len(r.Spec.Hostnames))
		}

		hostname := string(r.Spec.Hostnames[0])
		expectedName := gateway.GetStoreRouteName(store, hostname)
		if r.Name != expectedName {
			t.Errorf("expected route name %q, got %q", expectedName, r.Name)
		}

		// ParentRef points to gateway
		if len(r.Spec.ParentRefs) != 1 || string(r.Spec.ParentRefs[0].Name) != gateway.GetStoreGatewayName(store) {
			t.Errorf("expected ParentRef to %q, got %+v", gateway.GetStoreGatewayName(store), r.Spec.ParentRefs)
		}

		// Expect 4 rules (/api, /admin, /store-api, /)
		if len(r.Spec.Rules) != 4 {
			t.Errorf("expected 4 rules, got %d", len(r.Spec.Rules))
		}
	}
}
