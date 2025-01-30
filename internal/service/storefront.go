package service

import (
	"fmt"

	v1 "github.com/shopware/shopware-operator/api/v1"
	"github.com/shopware/shopware-operator/internal/util"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
)

func StorefrontService(store v1.Store) *corev1.Service {
	port := int32(8000)
	appName := "shopware-storefront"

	return &corev1.Service{
		TypeMeta: metav1.TypeMeta{
			APIVersion: "v1",
			Kind:       "Service",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      GetStorefrontServiceName(store),
			Namespace: store.Namespace,
			Labels:    util.GetDefaultStoreLabels(store),
		},
		Spec: corev1.ServiceSpec{
			Selector: map[string]string{
				"shop.shopware.com/store/app": appName,
			},
			Type:      corev1.ServiceTypeClusterIP,
			ClusterIP: "None",
			Ports: []corev1.ServicePort{
				{
					Protocol:   "TCP",
					Port:       port,
					TargetPort: intstr.FromInt32(port),
				},
			},
			PublishNotReadyAddresses: false,
		},
	}
}

func GetStorefrontServiceName(store v1.Store) string {
	return fmt.Sprintf("%s-storefront", store.Name)
}
