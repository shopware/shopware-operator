package service

import (
	"testing"

	"github.com/stretchr/testify/assert"
	corev1 "k8s.io/api/core/v1"
)

func names(ports []corev1.ServicePort) []string {
	out := make([]string, 0, len(ports))
	for _, p := range ports {
		out = append(out, p.Name)
	}
	return out
}

func TestServicePortsNames(t *testing.T) {
	tests := []struct {
		name     string
		ports    []corev1.ContainerPort
		reserved []string
		want     []string
	}{
		{
			name:  "unnamed ports get unique names",
			ports: []corev1.ContainerPort{{ContainerPort: 8080}, {ContainerPort: 8081}},
			want:  []string{"port-8080", "port-8081"},
		},
		{
			name:  "explicit names are kept",
			ports: []corev1.ContainerPort{{Name: "metrics", ContainerPort: 9090}, {ContainerPort: 8080}},
			want:  []string{"metrics", "port-8080"},
		},
		{
			name:  "generated name avoids an explicit name given later",
			ports: []corev1.ContainerPort{{ContainerPort: 8080}, {Name: "port-8080", ContainerPort: 9090}},
			want:  []string{"port-8080-2", "port-8080"},
		},
		{
			name: "same port number with different protocols",
			ports: []corev1.ContainerPort{
				{ContainerPort: 53, Protocol: corev1.ProtocolTCP},
				{ContainerPort: 53, Protocol: corev1.ProtocolUDP},
			},
			want: []string{"port-53", "port-53-2"},
		},
		{
			name:     "reserved names are avoided",
			ports:    []corev1.ContainerPort{{ContainerPort: 80}},
			reserved: []string{"port-80"},
			want:     []string{"port-80-2"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := servicePorts(tt.ports, tt.reserved...)
			assert.Equal(t, tt.want, names(got))
			for i, p := range got {
				assert.Equal(t, tt.ports[i].ContainerPort, p.Port)
				assert.Equal(t, tt.ports[i].ContainerPort, p.TargetPort.IntVal)
			}
		})
	}
}
