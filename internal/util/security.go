package util

import corev1 "k8s.io/api/core/v1"

// RestrictedContainerSecurityContext returns the fixed container-level settings
// required by the restricted Pod Security Standard. The CRD default only covers
// podSecurityContext, but allowPrivilegeEscalation and capabilities live on each
// container's securityContext.
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

// RestrictedPodSecurityContext mirrors the podSecurityContext default from the
// CRD for resources that were not API-server defaulted before reconciliation.
func RestrictedPodSecurityContext() *corev1.PodSecurityContext {
	runAsNonRoot := true

	return &corev1.PodSecurityContext{
		FSGroup:      Int64(82),
		RunAsGroup:   Int64(82),
		RunAsNonRoot: &runAsNonRoot,
		RunAsUser:    Int64(82),
		SeccompProfile: &corev1.SeccompProfile{
			Type: corev1.SeccompProfileTypeRuntimeDefault,
		},
	}
}

// DefaultPodSecurityContext keeps CRD-provided values unchanged and only falls
// back to the restricted defaults when podSecurityContext is missing.
func DefaultPodSecurityContext(securityContext *corev1.PodSecurityContext) *corev1.PodSecurityContext {
	if securityContext != nil {
		return securityContext
	}

	return RestrictedPodSecurityContext()
}
