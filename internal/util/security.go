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

// DefaultContainerSecurityContexts keeps user-provided container security
// contexts unchanged and only fills missing ones with the restricted defaults.
func DefaultContainerSecurityContexts(containers []corev1.Container) []corev1.Container {
	if containers == nil {
		return nil
	}

	defaulted := make([]corev1.Container, len(containers))
	copy(defaulted, containers)
	for i := range defaulted {
		if defaulted[i].SecurityContext == nil {
			defaulted[i].SecurityContext = RestrictedContainerSecurityContext()
		}
	}

	return defaulted
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
