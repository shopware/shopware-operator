package pod_test

import (
	"testing"

	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/utils/ptr"

	v1 "github.com/shopware/shopware-operator/api/v1"
	"github.com/shopware/shopware-operator/internal/pod"
	"github.com/shopware/shopware-operator/internal/util"
)

func TestTerminalDebugPod(t *testing.T) {
	instance := v1.TerminalDebugInstance{
		ObjectMeta: metav1.ObjectMeta{Name: "debug-1", Namespace: "app"},
		Spec: v1.TerminalDebugInstanceSpec{
			Container:   "web",
			CustomImage: "web:debug",
			ExtraContainerPorts: []corev1.ContainerPort{
				{Name: "http", ContainerPort: 3000},
				{Name: "grpc-terminal", ContainerPort: 8080},
			},
		},
	}
	source := corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "web-abc",
			Namespace: "app",
			Labels: map[string]string{
				"app.kubernetes.io/name": "web",
				"application-id":         "id-1",
				"pod-template-hash":      "abc",
			},
			Annotations:     map[string]string{"note": "kept"},
			OwnerReferences: []metav1.OwnerReference{{UID: "rs", Controller: ptr.To(true)}},
		},
		Spec: corev1.PodSpec{
			NodeName: "node-1",
			Containers: []corev1.Container{
				{
					Name:           "web",
					Image:          "web:1",
					Ports:          []corev1.ContainerPort{{Name: "http", ContainerPort: 3000}},
					LivenessProbe:  &corev1.Probe{},
					ReadinessProbe: &corev1.Probe{},
				},
				{Name: "sidecar", Image: "sidecar:1"},
			},
			RestartPolicy: corev1.RestartPolicyAlways,
		},
		Status: corev1.PodStatus{Phase: corev1.PodRunning},
	}
	selectorKeys := pod.ServiceSelectorKeys([]corev1.Service{
		{Spec: corev1.ServiceSpec{Selector: map[string]string{"app.kubernetes.io/name": "web"}}},
	})

	debugPod, container, err := pod.TerminalDebugPod(instance, source, selectorKeys)
	require.NoError(t, err)
	require.Equal(t, "web", container)

	require.Equal(t, "debug-1", debugPod.Name)
	require.Equal(t, "app", debugPod.Namespace)
	require.Empty(t, debugPod.OwnerReferences)
	require.Empty(t, debugPod.Status)
	require.Equal(t, map[string]string{
		"application-id":                            "id-1",
		util.TerminalDebugLabelKey:                  "true",
		"shop.shopware.com/terminal.debug.instance": "debug-1",
	}, debugPod.Labels)
	require.Equal(t, map[string]string{"note": "kept"}, debugPod.Annotations)

	require.Empty(t, debugPod.Spec.NodeName)
	require.Equal(t, corev1.RestartPolicyNever, debugPod.Spec.RestartPolicy)

	web := debugPod.Spec.Containers[0]
	require.Equal(t, "web:debug", web.Image)
	require.Nil(t, web.LivenessProbe)
	require.Nil(t, web.ReadinessProbe)
	require.Equal(t, []corev1.ContainerPort{
		{Name: "http", ContainerPort: 3000},
		{Name: "grpc-terminal", ContainerPort: 8080},
	}, web.Ports)
	require.Contains(t, web.VolumeMounts, corev1.VolumeMount{Name: "remote-terminal", MountPath: pod.TerminalMountPath})
	require.Equal(t, "sidecar:1", debugPod.Spec.Containers[1].Image)

	require.Equal(t, "web:1", source.Spec.Containers[0].Image, "source pod must not be mutated")
	require.NotNil(t, source.Spec.Containers[0].LivenessProbe, "source pod must not be mutated")
}

func TestTerminalDebugPodContainer(t *testing.T) {
	single := corev1.Pod{Spec: corev1.PodSpec{Containers: []corev1.Container{{Name: "app"}}}}
	multi := corev1.Pod{Spec: corev1.PodSpec{Containers: []corev1.Container{{Name: "app"}, {Name: "sidecar"}}}}

	tests := map[string]struct {
		source    corev1.Pod
		container string
		want      string
		wantErr   error
	}{
		"single container without name":    {source: single, want: "app"},
		"named container":                  {source: multi, container: "sidecar", want: "sidecar"},
		"multiple containers without name": {source: multi, wantErr: pod.ErrAmbiguousContainer},
		"unknown container":                {source: multi, container: "missing", wantErr: pod.ErrContainerNotFound},
	}
	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			instance := v1.TerminalDebugInstance{Spec: v1.TerminalDebugInstanceSpec{Container: tc.container}}
			_, got, err := pod.TerminalDebugPod(instance, tc.source, nil)
			if tc.wantErr != nil {
				require.ErrorIs(t, err, tc.wantErr)
				return
			}
			require.NoError(t, err)
			require.Equal(t, tc.want, got)
		})
	}
}

func TestSelectTerminalDebugSource(t *testing.T) {
	running := func(name string, owner types.UID, labels map[string]string) corev1.Pod {
		return corev1.Pod{
			ObjectMeta: metav1.ObjectMeta{
				Name:            name,
				Labels:          labels,
				OwnerReferences: []metav1.OwnerReference{{UID: owner, Controller: ptr.To(true)}},
			},
			Status: corev1.PodStatus{Phase: corev1.PodRunning},
		}
	}
	pending := running("a-pending", "rs", nil)
	pending.Status.Phase = corev1.PodPending
	debug := running("a-debug", "tdi", map[string]string{util.TerminalDebugLabelKey: "true"})

	tests := map[string]struct {
		pods    []corev1.Pod
		want    string
		wantErr error
	}{
		"no pods":               {wantErr: pod.ErrNoSourcePod},
		"only pending pods":     {pods: []corev1.Pod{pending}, wantErr: pod.ErrNoSourcePod},
		"only debug pods":       {pods: []corev1.Pod{debug}, wantErr: pod.ErrNoSourcePod},
		"multiple owners":       {pods: []corev1.Pod{running("a", "rs-1", nil), running("b", "rs-2", nil)}, wantErr: pod.ErrMultipleOwners},
		"first running by name": {pods: []corev1.Pod{pending, debug, running("c", "rs", nil), running("b", "rs", nil)}, want: "b"},
	}
	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			got, err := pod.SelectTerminalDebugSource(tc.pods)
			if tc.wantErr != nil {
				require.ErrorIs(t, err, tc.wantErr)
				return
			}
			require.NoError(t, err)
			require.Equal(t, tc.want, got.Name)
		})
	}
}
