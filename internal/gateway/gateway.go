package gateway

import (
	"context"
	"fmt"
	"maps"

	v1 "github.com/shopware/shopware-operator/api/v1"
	"github.com/shopware/shopware-operator/internal/service"
	"github.com/shopware/shopware-operator/internal/util"

	"sigs.k8s.io/controller-runtime/pkg/client"
	gwv1 "sigs.k8s.io/gateway-api/apis/v1"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
)

func GetStoreGateway(ctx context.Context, store v1.Store, client client.Client) (*gwv1.Gateway, error) {
	gw := StoreGateway(store)

	search := &gwv1.Gateway{
		ObjectMeta: gw.ObjectMeta,
	}

	err := client.Get(ctx, types.NamespacedName{
		Namespace: gw.Namespace,
		Name:      gw.Name,
	}, search)

	return search, err
}

func StoreGateway(store v1.Store) *gwv1.Gateway {
	labels := util.GetDefaultContainerStoreLabels(store, map[string]string{})
	maps.Copy(labels, store.Spec.Network.Labels)

	hosts := make([]string, len(store.Spec.Network.Hosts))
	_ = copy(hosts, store.Spec.Network.Hosts)

	if store.Spec.Network.Host != "" {
		hosts = append(hosts, store.Spec.Network.Host)
	}

	gw := &gwv1.Gateway{
		ObjectMeta: metav1.ObjectMeta{
			Name:        GetStoreGatewayName(store),
			Namespace:   store.Namespace,
			Annotations: store.Spec.Network.Annotations,
			Labels:      labels,
		},
		Spec: gwv1.GatewaySpec{
			GatewayClassName: gwv1.ObjectName(store.Spec.Network.IngressClassName),
			Listeners: []gwv1.Listener{
				{
					Name:     "http",
					Protocol: gwv1.HTTPProtocolType,
					Port:     80,
				},
			},
		},
	}

	// TLS optional
	if store.Spec.Network.TLSSecretName != "" {
		mode := gwv1.TLSModeTerminate
		gw.Spec.Listeners = append(gw.Spec.Listeners, gwv1.Listener{
			Name:     "https",
			Protocol: gwv1.HTTPSProtocolType,
			Port:     443,
			TLS: &gwv1.GatewayTLSConfig{
				Mode: &mode,
				CertificateRefs: []gwv1.SecretObjectReference{
					{
						Name: gwv1.ObjectName(store.Spec.Network.TLSSecretName),
					},
				},
			},
		})
	}

	return gw
}

func StoreHTTPRoutes(store v1.Store) []*gwv1.HTTPRoute {
	hosts := make([]string, len(store.Spec.Network.Hosts))
	_ = copy(hosts, store.Spec.Network.Hosts)

	if store.Spec.Network.Host != "" {
		hosts = append(hosts, store.Spec.Network.Host)
	}

	var routes []*gwv1.HTTPRoute

	for _, host := range hosts {
		route := &gwv1.HTTPRoute{
			ObjectMeta: metav1.ObjectMeta{
				Name:        GetStoreRouteName(store, host),
				Namespace:   store.Namespace,
				Annotations: store.Spec.Network.Annotations,
			},
			Spec: gwv1.HTTPRouteSpec{
				Hostnames: []gwv1.Hostname{gwv1.Hostname(host)},
				CommonRouteSpec: gwv1.CommonRouteSpec{
					ParentRefs: []gwv1.ParentReference{
						{
							Name: gwv1.ObjectName(GetStoreGatewayName(store)),
						},
					},
				},
				Rules: []gwv1.HTTPRouteRule{
					routeRule("/api", service.GetAdminServiceName(store), store.Spec.Network.Port),
					routeRule("/admin", service.GetAdminServiceName(store), store.Spec.Network.Port),
					routeRule("/store-api", service.GetStorefrontServiceName(store), store.Spec.Network.Port),
					routeRule("/", service.GetStorefrontServiceName(store), store.Spec.Network.Port),
				},
			},
		}

		routes = append(routes, route)
	}

	return routes
}

func routeRule(path, svc string, port int32) gwv1.HTTPRouteRule {
	pt := gwv1.PathMatchPathPrefix
	gwport := gwv1.PortNumber(port)

	return gwv1.HTTPRouteRule{
		Matches: []gwv1.HTTPRouteMatch{
			{
				Path: &gwv1.HTTPPathMatch{
					Type:  &pt,
					Value: &path,
				},
			},
		},
		BackendRefs: []gwv1.HTTPBackendRef{
			{
				BackendRef: gwv1.BackendRef{
					BackendObjectReference: gwv1.BackendObjectReference{
						Name: gwv1.ObjectName(svc),
						Port: &gwport,
					},
				},
			},
		},
	}
}

func GetStoreGatewayName(store v1.Store) string {
	return fmt.Sprintf("store-%s-gw", store.Name)
}

func GetStoreRouteName(store v1.Store, host string) string {
	return fmt.Sprintf("store-%s-%s-route", store.Name, util.SafeDNSName(host))
}
