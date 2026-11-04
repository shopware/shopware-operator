package service

import (
	v1 "github.com/shopware/shopware-operator/api/v1"
	"github.com/shopware/shopware-operator/internal/util"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
)

// TerminalDebugService exposes the extra container ports of the debug pod
// under the name of its TerminalDebugInstance.
func TerminalDebugService(instance v1.TerminalDebugInstance) *corev1.Service {
	ports := make([]corev1.ServicePort, 0, len(instance.Spec.ExtraContainerPorts))
	for _, port := range instance.Spec.ExtraContainerPorts {
		ports = append(ports, corev1.ServicePort{
			Name:       port.Name,
			Protocol:   port.Protocol,
			Port:       port.ContainerPort,
			TargetPort: intstr.FromInt32(port.ContainerPort),
		})
	}

	labels := util.GetTerminalDebugInstanceLabels(instance)

	return &corev1.Service{
		TypeMeta: metav1.TypeMeta{
			APIVersion: "v1",
			Kind:       "Service",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      instance.Name,
			Namespace: instance.Namespace,
			Labels:    labels,
		},
		Spec: corev1.ServiceSpec{
			Selector: labels,
			Type:     corev1.ServiceTypeClusterIP,
			Ports:    ports,
		},
	}
}
