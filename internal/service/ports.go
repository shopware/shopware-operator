package service

import (
	"fmt"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
)

// servicePorts converts container ports to service ports. Container port names
// are optional, but a Service with several ports needs a unique, non-empty name
// for each of them. Unnamed ports get a generated name ("port-<number>") that
// avoids every explicitly supplied name and every name in reserved, such as the
// names of ports that are already part of the Service. Explicit names are kept
// as given. The result is deterministic, so it does not cause spurious updates.
func servicePorts(ports []corev1.ContainerPort, reserved ...string) []corev1.ServicePort {
	used := make(map[string]struct{}, len(ports)+len(reserved))
	for _, name := range reserved {
		used[name] = struct{}{}
	}
	for _, port := range ports {
		if port.Name != "" {
			used[port.Name] = struct{}{}
		}
	}

	result := make([]corev1.ServicePort, 0, len(ports))
	for _, port := range ports {
		name := port.Name
		if name == "" {
			name = uniquePortName(fmt.Sprintf("port-%d", port.ContainerPort), used)
			used[name] = struct{}{}
		}
		result = append(result, corev1.ServicePort{
			Name:       name,
			Protocol:   port.Protocol,
			Port:       port.ContainerPort,
			TargetPort: intstr.FromInt32(port.ContainerPort),
		})
	}
	return result
}

func uniquePortName(base string, used map[string]struct{}) string {
	name := base
	for i := 2; ; i++ {
		if _, taken := used[name]; !taken {
			return name
		}
		name = fmt.Sprintf("%s-%d", base, i)
	}
}
