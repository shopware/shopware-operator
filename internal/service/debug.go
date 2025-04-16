package service

import (
	v1 "github.com/shopware/shopware-operator/api/v1"
	"github.com/shopware/shopware-operator/internal/util"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
)

func DebugService(store v1.Store, debugInstance v1.StoreDebugInstance) *corev1.Service {
	ports := []corev1.ServicePort{
		{
			Name:       "http",
			Protocol:   "TCP",
			Port:       store.Spec.Container.Port,
			TargetPort: intstr.FromInt32(store.Spec.Container.Port),
		},
	}

	// Add any extra ports from the debug instance
	for _, port := range debugInstance.Spec.ExtraContainerPorts {
		ports = append(ports, corev1.ServicePort{
			Name:       port.Name,
			Protocol:   port.Protocol,
			Port:       port.ContainerPort,
			TargetPort: intstr.FromInt32(port.ContainerPort),
		})
	}

	selector := util.GetDefaultStoreInstanceDebugLabels(store, debugInstance)

	return &corev1.Service{
		TypeMeta: metav1.TypeMeta{
			APIVersion: "v1",
			Kind:       "Service",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      debugInstance.Name,
			Namespace: store.Namespace,
			Labels:    util.GetDefaultStoreLabels(store),
		},
		Spec: corev1.ServiceSpec{
			Selector: selector,
			Type:     corev1.ServiceTypeClusterIP,
			Ports:    ports,
		},
	}
}
