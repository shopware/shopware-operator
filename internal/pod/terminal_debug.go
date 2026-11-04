package pod

import (
	"cmp"
	"errors"
	"fmt"
	"maps"
	"slices"
	"strconv"
	"strings"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"

	v1 "github.com/shopware/shopware-operator/api/v1"
	"github.com/shopware/shopware-operator/internal/util"
)

const (
	terminalVolumeName = "remote-terminal"
	// TerminalMountPath is writable in every debug container, so the terminal
	// server can be copied into images that have no writable path of their own.
	TerminalMountPath = "/terminal"

	deploymentRevisionAnnotation = "deployment.kubernetes.io/revision"
)

var (
	ErrNoSourcePod        = errors.New("no running pod matches the pod selector")
	ErrMultipleWorkloads  = errors.New("running pods matching the pod selector belong to more than one workload")
	ErrContainerNotFound  = errors.New("container not found in the selected pod")
	ErrAmbiguousContainer = errors.New("container must be set because the selected pod has more than one container")
)

// controllerLabelKeys are set by workload controllers on the pods they own.
// The debug pod is owned by its TerminalDebugInstance and does not carry them.
var controllerLabelKeys = []string{
	"pod-template-hash",
	"controller-revision-hash",
	"statefulset.kubernetes.io/pod-name",
	"apps.kubernetes.io/pod-index",
	"batch.kubernetes.io/controller-uid",
	"batch.kubernetes.io/job-name",
	"controller-uid",
	"job-name",
}

// terminalDebugCandidate is a source pod with the workload it belongs to.
type terminalDebugCandidate struct {
	pod corev1.Pod
	// workload is the UID of the top-level controller: the Deployment for
	// ReplicaSet pods, the direct controller otherwise, empty for bare pods.
	workload types.UID
	// revision orders the ReplicaSets of one Deployment, zero otherwise.
	revision int64
}

func newTerminalDebugCandidate(p corev1.Pod, replicaSets map[types.UID]appsv1.ReplicaSet) terminalDebugCandidate {
	candidate := terminalDebugCandidate{pod: p}
	ref := metav1.GetControllerOf(&p)
	if ref == nil {
		return candidate
	}
	candidate.workload = ref.UID
	if ref.Kind != "ReplicaSet" {
		return candidate
	}

	rs, ok := replicaSets[ref.UID]
	if !ok {
		return candidate
	}
	if deployment := metav1.GetControllerOf(&rs); deployment != nil {
		candidate.workload = deployment.UID
	}
	candidate.revision, _ = strconv.ParseInt(rs.Annotations[deploymentRevisionAnnotation], 10, 64)
	return candidate
}

// SelectTerminalDebugSource picks the running pod to copy from the pods that
// match the instance pod selector. Debug pods are never copied. Pods of one
// Deployment count as one workload, and only the newest ReplicaSet is copied,
// so a rolling update does not look like two workloads.
func SelectTerminalDebugSource(pods []corev1.Pod, replicaSets []appsv1.ReplicaSet) (corev1.Pod, error) {
	replicaSetsByUID := make(map[types.UID]appsv1.ReplicaSet, len(replicaSets))
	for _, rs := range replicaSets {
		replicaSetsByUID[rs.UID] = rs
	}

	candidates := make([]terminalDebugCandidate, 0, len(pods))
	workloads := make(map[types.UID]struct{})
	for _, p := range pods {
		if p.Status.Phase != corev1.PodRunning || p.DeletionTimestamp != nil {
			continue
		}
		if _, debug := p.Labels[util.TerminalDebugLabelKey]; debug {
			continue
		}
		candidate := newTerminalDebugCandidate(p, replicaSetsByUID)
		workloads[candidate.workload] = struct{}{}
		candidates = append(candidates, candidate)
	}

	if len(candidates) == 0 {
		return corev1.Pod{}, ErrNoSourcePod
	}
	if len(workloads) > 1 {
		return corev1.Pod{}, ErrMultipleWorkloads
	}

	newest := slices.MaxFunc(candidates, func(a, b terminalDebugCandidate) int {
		return cmp.Compare(a.revision, b.revision)
	}).revision
	candidates = slices.DeleteFunc(candidates, func(c terminalDebugCandidate) bool {
		return c.revision != newest
	})

	slices.SortFunc(candidates, func(a, b terminalDebugCandidate) int {
		return strings.Compare(a.pod.Name, b.pod.Name)
	})
	return candidates[0].pod, nil
}

// ServiceSelectorKeys returns every label key used by a Service selector.
func ServiceSelectorKeys(services []corev1.Service) map[string]struct{} {
	keys := make(map[string]struct{})
	for _, svc := range services {
		for key := range svc.Spec.Selector {
			keys[key] = struct{}{}
		}
	}
	return keys
}

// TerminalDebugPod copies source into a debug pod for the instance. Labels a
// Service selects on are dropped, so the debug pod receives no live traffic.
// It returns the pod and the name of the debug container.
func TerminalDebugPod(
	instance v1.TerminalDebugInstance,
	source corev1.Pod,
	serviceSelectorKeys map[string]struct{},
) (*corev1.Pod, string, error) {
	spec := *source.Spec.DeepCopy()

	index, err := debugContainerIndex(spec.Containers, instance.Spec.Container)
	if err != nil {
		return nil, "", err
	}

	container := &spec.Containers[index]
	if instance.Spec.CustomImage != "" {
		container.Image = instance.Spec.CustomImage
	}
	// The debug pod keeps running even when the copied workload is unhealthy.
	container.LivenessProbe = nil
	container.ReadinessProbe = nil
	container.StartupProbe = nil
	for _, port := range instance.Spec.ExtraContainerPorts {
		if !slices.ContainsFunc(container.Ports, func(p corev1.ContainerPort) bool {
			return p.ContainerPort == port.ContainerPort
		}) {
			container.Ports = append(container.Ports, port)
		}
	}
	container.VolumeMounts = append(container.VolumeMounts, corev1.VolumeMount{
		Name:      terminalVolumeName,
		MountPath: TerminalMountPath,
	})
	spec.Volumes = append(spec.Volumes, corev1.Volume{
		Name: terminalVolumeName,
		VolumeSource: corev1.VolumeSource{
			EmptyDir: &corev1.EmptyDirVolumeSource{},
		},
	})

	spec.NodeName = ""
	spec.Hostname = ""
	spec.Subdomain = ""
	spec.EphemeralContainers = nil
	spec.RestartPolicy = corev1.RestartPolicyNever

	labels := make(map[string]string, len(source.Labels)+2)
	for key, value := range source.Labels {
		if _, selected := serviceSelectorKeys[key]; selected {
			continue
		}
		if slices.Contains(controllerLabelKeys, key) {
			continue
		}
		labels[key] = value
	}
	maps.Copy(labels, util.GetTerminalDebugInstanceLabels(instance))

	return &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:        instance.Name,
			Namespace:   instance.Namespace,
			Labels:      labels,
			Annotations: maps.Clone(source.Annotations),
		},
		Spec: spec,
	}, container.Name, nil
}

// DebugContainerName returns the name of the container that is the debug
// container of p: the requested one, or the sole container when none is
// requested. It applies the same selection as TerminalDebugPod. The debug pod
// keeps the container names of its source pod, so the name can be recovered
// from an existing debug pod.
func DebugContainerName(p corev1.Pod, requested string) (string, error) {
	index, err := debugContainerIndex(p.Spec.Containers, requested)
	if err != nil {
		return "", err
	}
	return p.Spec.Containers[index].Name, nil
}

func debugContainerIndex(containers []corev1.Container, name string) (int, error) {
	if name == "" {
		if len(containers) != 1 {
			return 0, ErrAmbiguousContainer
		}
		return 0, nil
	}
	for i := range containers {
		if containers[i].Name == name {
			return i, nil
		}
	}
	return 0, fmt.Errorf("%w: %s", ErrContainerNotFound, name)
}
