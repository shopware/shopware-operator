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

	var tls []networkingv1.IngressTLS
	if store.Spec.Network.TLSSecretName != "" {
		tls = append(tls, networkingv1.IngressTLS{
			Hosts: []string{
				store.Spec.Network.Host,
			},
			SecretName: store.Spec.Network.TLSSecretName,
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
			Rules: []networkingv1.IngressRule{
				{
					Host: store.Spec.Network.Host,
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
				},
			},
			TLS: tls,
		},
	}
}

func GetStoreIngressName(store v1.Store) string {
	return fmt.Sprintf("store-%s", store.Name)
}
