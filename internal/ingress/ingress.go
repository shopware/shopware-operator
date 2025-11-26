package ingress

import (
	"context"
	"fmt"
	"maps"

	v1 "github.com/shopware/shopware-operator/api/v1"
	"github.com/shopware/shopware-operator/internal/service"
	"github.com/shopware/shopware-operator/internal/util"
	appsv1 "k8s.io/api/apps/v1"
	networkingv1 "k8s.io/api/networking/v1"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

func GetStoreIngress(
	ctx context.Context,
	store v1.Store,
	client client.Client,
) (*appsv1.Deployment, error) {
	ingress := StoreIngress(store)
	search := &appsv1.Deployment{
		ObjectMeta: ingress.ObjectMeta,
	}
	err := client.Get(ctx, types.NamespacedName{
		Namespace: ingress.Namespace,
		Name:      ingress.Name,
	}, search)
	return search, err
}

func StoreIngress(store v1.Store) *networkingv1.Ingress {
	pathType := networkingv1.PathTypePrefix

	labels := util.GetDefaultContainerStoreLabels(store, map[string]string{})
	maps.Copy(labels, store.Spec.Network.Labels)

	hosts := make([]string, len(store.Spec.Network.Hosts))
	_ = copy(hosts, store.Spec.Network.Hosts)

	if store.Spec.Network.Host != "" {
		hosts = append(hosts, store.Spec.Network.Host)
	}

	var tls []networkingv1.IngressTLS
	if store.Spec.Network.TLSSecretName != "" {
		tls = append(tls, networkingv1.IngressTLS{
			Hosts:      hosts,
			SecretName: store.Spec.Network.TLSSecretName,
		})
	}

	rules := make([]networkingv1.IngressRule, 0, len(hosts))
	for _, host := range hosts {
		rules = append(rules, networkingv1.IngressRule{
			Host: host,
			IngressRuleValue: networkingv1.IngressRuleValue{
				HTTP: &networkingv1.HTTPIngressRuleValue{
					Paths: []networkingv1.HTTPIngressPath{
						{
							Path:     "/api",
							PathType: &pathType,
							Backend: networkingv1.IngressBackend{
								Service: &networkingv1.IngressServiceBackend{
									Name: service.GetAdminServiceName(store),
									Port: networkingv1.ServiceBackendPort{
										Number: store.Spec.Network.Port,
									},
								},
							},
						},
						{
							Path:     "/admin",
							PathType: &pathType,
							Backend: networkingv1.IngressBackend{
								Service: &networkingv1.IngressServiceBackend{
									Name: service.GetAdminServiceName(store),
									Port: networkingv1.ServiceBackendPort{
										Number: store.Spec.Network.Port,
									},
								},
							},
						},
						{
							Path:     "/store-api",
							PathType: &pathType,
							Backend: networkingv1.IngressBackend{
								Service: &networkingv1.IngressServiceBackend{
									Name: service.GetStorefrontServiceName(store),
									Port: networkingv1.ServiceBackendPort{
										Number: store.Spec.Network.Port,
									},
								},
							},
						},
						{
							Path:     "/",
							PathType: &pathType,
							Backend: networkingv1.IngressBackend{
								Service: &networkingv1.IngressServiceBackend{
									Name: service.GetStorefrontServiceName(store),
									Port: networkingv1.ServiceBackendPort{
										Number: store.Spec.Network.Port,
									},
								},
							},
						},
					},
				},
			},
		})
	}

	return &networkingv1.Ingress{
		TypeMeta: metav1.TypeMeta{},
		ObjectMeta: metav1.ObjectMeta{
			Name:        GetStoreIngressName(store),
			Namespace:   store.GetNamespace(),
			Annotations: store.Spec.Network.Annotations,
			Labels:      labels,
		},
		Spec: networkingv1.IngressSpec{
			IngressClassName: &store.Spec.Network.IngressClassName,
			Rules:            rules,
			TLS:              tls,
		},
	}
}

func GetStoreIngressName(store v1.Store) string {
	return fmt.Sprintf("store-%s", store.Name)
}

func DeleteStoreIngress(ctx context.Context, c client.Client, store v1.Store) error {
	ingress := &networkingv1.Ingress{}
	err := c.Get(ctx, types.NamespacedName{
		Namespace: store.GetNamespace(),
		Name:      GetStoreIngressName(store),
	}, ingress)
	if err != nil {
		if k8serrors.IsNotFound(err) {
			return nil
		}
		return err
	}

	return c.Delete(ctx, ingress, client.PropagationPolicy("Foreground"))
}
