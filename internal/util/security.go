package util

import corev1 "k8s.io/api/core/v1"

func RestrictedContainerSecurityContext() *corev1.SecurityContext {
	allowPrivilegeEscalation := false

	return &corev1.SecurityContext{
		AllowPrivilegeEscalation: &allowPrivilegeEscalation,
		Capabilities: &corev1.Capabilities{
			Drop: []corev1.Capability{
				"ALL",
			},
		},
	}
}
