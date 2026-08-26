package deployment

import (
	"context"
	"fmt"
	"maps"
	"math"

	v1 "github.com/shopware/shopware-operator/api/v1"
	"github.com/shopware/shopware-operator/internal/util"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/intstr"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

func GetWorkerDeployment(
	ctx context.Context,
	store v1.Store,
	client client.Client,
) (*appsv1.Deployment, error) {
	setup := WorkerDeployment(store)
	search := &appsv1.Deployment{
		ObjectMeta: setup.ObjectMeta,
	}
	err := client.Get(ctx, types.NamespacedName{
		Namespace: setup.Namespace,
		Name:      setup.Name,
	}, search)
	return search, err
}

func GetWorkerDeploymentCondition(
	ctx context.Context,
	store v1.Store,
	client client.Client,
) v1.DeploymentCondition {
	deployment := WorkerDeployment(store)
	search := &appsv1.Deployment{
		ObjectMeta: deployment.ObjectMeta,
	}
	err := client.Get(ctx, types.NamespacedName{
		Namespace: deployment.Namespace,
		Name:      deployment.Name,
	}, search)
	if err != nil {
		if k8serrors.IsNotFound(err) {
			return v1.DeploymentCondition{
				State:          v1.DeploymentStateNotFound,
				LastUpdateTime: metav1.Now(),
				Message:        "No deployment found",
				Ready:          "0/0",
			}
		} else {
			return v1.DeploymentCondition{
				State:          v1.DeploymentStateError,
				LastUpdateTime: metav1.Now(),
				Message:        fmt.Sprintf("error on client get: %s", err),
				Ready:          "0/0",
			}
		}
	}
	return getDeploymentCondition(search, *deployment.Spec.Replicas)
}

func WorkerDeployment(store v1.Store) *appsv1.Deployment {
	containerSpec := store.Spec.Container.DeepCopy()
	containerSpec.Merge(store.Spec.WorkerDeploymentContainer)
	containerSpec.Volumes = append(containerSpec.Volumes, store.GetDatabaseTLSVolumes()...)
	containerSpec.VolumeMounts = append(containerSpec.VolumeMounts, store.GetDatabaseTLSVolumeMounts()...)

	appName := "shopware-worker"
	labels := util.GetDefaultContainerStoreLabels(store, store.Spec.WorkerDeploymentContainer.Labels)
	maps.Copy(labels, util.GetWorkerDeploymentMatchLabel())

	annotations := util.GetDefaultContainerAnnotations(appName, store, store.Spec.WorkerDeploymentContainer.Annotations)

	// Worker-specific defaults layered over the shared env, still overridable by
	// ExtraEnvs. The worker runs a few long-lived processes, so persistent DB
	// connections are beneficial here (storefront/admin default to 0 in GetEnv to
	// avoid hoarding connections across many short-lived requests).
	workerDefaults := []corev1.EnvVar{
		{Name: "DATABASE_PERSISTENT_CONNECTION", Value: "1"},
	}
	envs := util.MergeEnv(util.MergeEnv(store.GetEnv(), workerDefaults), containerSpec.ExtraEnvs)

	// Set PHP_MEMORY_LIMIT to 90% of the container memory limit
	phpMemoryLimitMiB := 0
	if containerSpec.Resources.Limits.Memory() != nil && containerSpec.Resources.Limits.Memory().Value() != 0 {
		memoryLimitMiB := containerSpec.Resources.Limits.Memory().Value() / (1024 * 1024)
		phpMemoryLimitMiB = int(math.Floor(float64(memoryLimitMiB) * 0.9))
		envs = util.MergeEnv(envs, []corev1.EnvVar{
			{
				Name:  "PHP_MEMORY_LIMIT",
				Value: fmt.Sprintf("%dM", phpMemoryLimitMiB),
			},
		})
	}

	consume := "bin/console messenger:consume failed async low_priority webhook --time-limit=300"
	if phpMemoryLimitMiB > 0 {
		consume += fmt.Sprintf(" --memory-limit=%dM", phpMemoryLimitMiB)
	}
	workerScript := fmt.Sprintf(
		`trap 'kill -TERM "$child" 2>/dev/null' TERM INT
while true; do
  %s &
  child=$!
  wait "$child"
  [ $? -gt 128 ] && exit 0
done`,
		consume,
	)

	containers := append(util.DefaultContainerSecurityContexts(containerSpec.ExtraContainers), corev1.Container{
		Name:            appName,
		Image:           containerSpec.Image,
		ImagePullPolicy: containerSpec.ImagePullPolicy,
		Env:             envs,
		SecurityContext: util.RestrictedContainerSecurityContext(),
		Command: []string{
			"/bin/sh",
			"-c",
		},
		Args: []string{
			workerScript,
		},
		VolumeMounts: containerSpec.VolumeMounts,
		Ports: []corev1.ContainerPort{
			{
				ContainerPort: containerSpec.Port,
				Protocol:      corev1.ProtocolTCP,
			},
		},
		Resources: containerSpec.Resources,
	})

	deployment := &appsv1.Deployment{
		TypeMeta: metav1.TypeMeta{
			Kind:       "Deployment",
			APIVersion: "apps/v1",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:        GetWorkerDeploymentName(store),
			Namespace:   store.Namespace,
			Labels:      labels,
			Annotations: annotations,
		},
		Spec: appsv1.DeploymentSpec{
			ProgressDeadlineSeconds: &containerSpec.ProgressDeadlineSeconds,
			Replicas:                &containerSpec.Replicas,
			Selector: &metav1.LabelSelector{
				MatchLabels: util.GetWorkerDeploymentMatchLabel(),
			},
			Strategy: appsv1.DeploymentStrategy{
				RollingUpdate: &appsv1.RollingUpdateDeployment{
					MaxSurge: &intstr.IntOrString{
						Type:   intstr.String,
						StrVal: "25%",
					},
					MaxUnavailable: &intstr.IntOrString{
						Type:   intstr.String,
						StrVal: "25%",
					},
				},
			},
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{
					Labels:      labels,
					Annotations: annotations,
				},
				Spec: corev1.PodSpec{
					Volumes:                   containerSpec.Volumes,
					TopologySpreadConstraints: containerSpec.TopologySpreadConstraints,
					NodeSelector:              containerSpec.NodeSelector,
					ImagePullSecrets:          containerSpec.ImagePullSecrets,
					EnableServiceLinks:        containerSpec.EnableServiceLinks,
					RestartPolicy:             containerSpec.RestartPolicy,
					Containers:                containers,
					SecurityContext:           util.DefaultPodSecurityContext(containerSpec.SecurityContext),
					InitContainers:            util.DefaultContainerSecurityContexts(containerSpec.InitContainers),
				},
			},
		},
	}

	// Old way
	if store.Spec.ServiceAccountName != "" {
		deployment.Spec.Template.Spec.ServiceAccountName = store.Spec.ServiceAccountName
	}
	// New way
	if containerSpec.ServiceAccountName != "" {
		deployment.Spec.Template.Spec.ServiceAccountName = containerSpec.ServiceAccountName
	}

	return deployment
}

func GetWorkerDeploymentName(store v1.Store) string {
	return fmt.Sprintf("%s-store-worker", store.Name)
}
