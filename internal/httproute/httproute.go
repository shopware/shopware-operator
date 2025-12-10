package httproute

import (
	"context"
	"fmt"
	"maps"

	v1 "github.com/shopware/shopware-operator/api/v1"
	"github.com/shopware/shopware-operator/internal/service"
	"github.com/shopware/shopware-operator/internal/util"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	gatewayv1 "sigs.k8s.io/gateway-api/apis/v1"
)

func GetStoreHTTPRoute(
	ctx context.Context,
	store v1.Store,
	client client.Client,
) (*gatewayv1.HTTPRoute, error) {
	httpRoute := StoreHTTPRoute(store)
	search := &gatewayv1.HTTPRoute{
		ObjectMeta: httpRoute.ObjectMeta,
	}
	err := client.Get(ctx, types.NamespacedName{
		Namespace: httpRoute.Namespace,
		Name:      httpRoute.Name,
	}, search)
	return search, err
}

func StoreHTTPRoute(store v1.Store) *gatewayv1.HTTPRoute {
	pathMatchPrefix := gatewayv1.PathMatchPathPrefix

	labels := util.GetDefaultContainerStoreLabels(store, map[string]string{})
	maps.Copy(labels, store.Spec.Network.Labels)

	hosts := make([]string, len(store.Spec.Network.Hosts))
	_ = copy(hosts, store.Spec.Network.Hosts)

	if store.Spec.Network.Host != "" {
		hosts = append(hosts, store.Spec.Network.Host)
	}

	hostnames := make([]gatewayv1.Hostname, 0, len(hosts))
	for _, host := range hosts {
		hostname := gatewayv1.Hostname(host)
		hostnames = append(hostnames, hostname)
	}

	adminServiceName := gatewayv1.ObjectName(service.GetAdminServiceName(store))
	storefrontServiceName := gatewayv1.ObjectName(service.GetStorefrontServiceName(store))
	port := gatewayv1.PortNumber(store.Spec.Network.Port)

	rules := []gatewayv1.HTTPRouteRule{
		{
			Matches: []gatewayv1.HTTPRouteMatch{
				{
					Path: &gatewayv1.HTTPPathMatch{
						Type:  &pathMatchPrefix,
						Value: util.StrPtr("/api"),
					},
				},
			},
			BackendRefs: []gatewayv1.HTTPBackendRef{
				{
					BackendRef: gatewayv1.BackendRef{
						BackendObjectReference: gatewayv1.BackendObjectReference{
							Name: adminServiceName,
							Port: &port,
						},
					},
				},
			},
		},
		{
			Matches: []gatewayv1.HTTPRouteMatch{
				{
					Path: &gatewayv1.HTTPPathMatch{
						Type:  &pathMatchPrefix,
						Value: util.StrPtr("/admin"),
					},
				},
			},
			BackendRefs: []gatewayv1.HTTPBackendRef{
				{
					BackendRef: gatewayv1.BackendRef{
						BackendObjectReference: gatewayv1.BackendObjectReference{
							Name: adminServiceName,
							Port: &port,
						},
					},
				},
			},
		},
		{
			Matches: []gatewayv1.HTTPRouteMatch{
				{
					Path: &gatewayv1.HTTPPathMatch{
						Type:  &pathMatchPrefix,
						Value: util.StrPtr("/store-api"),
					},
				},
			},
			BackendRefs: []gatewayv1.HTTPBackendRef{
				{
					BackendRef: gatewayv1.BackendRef{
						BackendObjectReference: gatewayv1.BackendObjectReference{
							Name: storefrontServiceName,
							Port: &port,
						},
					},
				},
			},
		},
		{
			Matches: []gatewayv1.HTTPRouteMatch{
				{
					Path: &gatewayv1.HTTPPathMatch{
						Type:  &pathMatchPrefix,
						Value: util.StrPtr("/"),
					},
				},
			},
			BackendRefs: []gatewayv1.HTTPBackendRef{
				{
					BackendRef: gatewayv1.BackendRef{
						BackendObjectReference: gatewayv1.BackendObjectReference{
							Name: storefrontServiceName,
							Port: &port,
						},
					},
				},
			},
		},
	}

	parentRef := gatewayv1.ParentReference{
		Name: gatewayv1.ObjectName(store.Spec.Network.GatewayName),
	}

	if store.Spec.Network.GatewayNamespace != "" {
		namespace := gatewayv1.Namespace(store.Spec.Network.GatewayNamespace)
		parentRef.Namespace = &namespace
	}

	if store.Spec.Network.GatewaySectionName != "" {
		sectionName := gatewayv1.SectionName(store.Spec.Network.GatewaySectionName)
		parentRef.SectionName = &sectionName
	}

	parentRefs := []gatewayv1.ParentReference{parentRef}

	return &gatewayv1.HTTPRoute{
		TypeMeta: metav1.TypeMeta{
			APIVersion: "gateway.networking.k8s.io/v1",
			Kind:       "HTTPRoute",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:        GetStoreHTTPRouteName(store),
			Namespace:   store.GetNamespace(),
			Annotations: store.Spec.Network.Annotations,
			Labels:      labels,
		},
		Spec: gatewayv1.HTTPRouteSpec{
			CommonRouteSpec: gatewayv1.CommonRouteSpec{
				ParentRefs: parentRefs,
			},
			Hostnames: hostnames,
			Rules:     rules,
		},
	}
}

func GetStoreHTTPRouteName(store v1.Store) string {
	return fmt.Sprintf("store-%s", store.Name)
}

func DeleteStoreHTTPRoute(ctx context.Context, c client.Client, store v1.Store) error {
	httpRoute := &gatewayv1.HTTPRoute{}
	err := c.Get(ctx, types.NamespacedName{
		Namespace: store.GetNamespace(),
		Name:      GetStoreHTTPRouteName(store),
	}, httpRoute)
	if err != nil {
		if k8serrors.IsNotFound(err) {
			return nil
		}
		return err
	}

	return c.Delete(ctx, httpRoute, client.PropagationPolicy("Foreground"))
}
