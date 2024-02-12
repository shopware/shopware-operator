package service

import (
	"fmt"

	v1 "github.com/shopware/shopware-operator/api/v1"
	"github.com/shopware/shopware-operator/internal/util"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
)

func StoreService(store *v1.Store) *corev1.Service {
	port := int32(8000)
	appName := "shopware"

	return &corev1.Service{
		TypeMeta: metav1.TypeMeta{
			APIVersion: "v1",
			Kind:       "Service",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      ServiceName(store),
			Namespace: store.Namespace,
			Labels:    util.GetDefaultLabels(store),
		},
		Spec: corev1.ServiceSpec{
			Selector: map[string]string{
				"app": appName,
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

func ServiceName(store *v1.Store) string {
	return fmt.Sprintf("store-%s", store.Name)
}
