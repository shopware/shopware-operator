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

const (
	DEPLOYMENT_STOREFRONT_CONTAINER_NAME = "shopware-storefront"
	startServersRatio                    = 0.25
	minSpareServersRatio                 = 0.2
	maxSpareServersRatio                 = 0.375
	memoryPerChildMiB                    = 80 //Every PHP-FPM process in an empty shop uses 70.6MiB
)

var memoryLimitMiB, maxSpare, minSpare, startServers, maxChildren int

func GetStorefrontDeployment(
	ctx context.Context,
	store v1.Store,
	client client.Client,
) (*appsv1.Deployment, error) {
	setup := StorefrontDeployment(store)
	search := &appsv1.Deployment{
		ObjectMeta: setup.ObjectMeta,
	}
	err := client.Get(ctx, types.NamespacedName{
		Namespace: setup.Namespace,
		Name:      setup.Name,
	}, search)
	return search, err
}

func GetStorefrontDeploymentCondition(
	ctx context.Context,
	store v1.Store,
	client client.Client,
) v1.DeploymentCondition {
	deployment := StorefrontDeployment(store)
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

func StorefrontDeployment(store v1.Store) *appsv1.Deployment {
	containerSpec := store.Spec.Container.DeepCopy()
	containerSpec.Merge(store.Spec.StorefrontDeploymentContainer)

	// think of the debug container to when changing the deployment

	appName := "shopware-storefront"
	labels := util.GetDefaultContainerStoreLabels(store, store.Spec.StorefrontDeploymentContainer.Labels)
	maps.Copy(labels, util.GetStorefrontDeploymentMatchLabel())

	annotations := util.GetDefaultContainerAnnotations(appName, store, store.Spec.StorefrontDeploymentContainer.Annotations)

	// Merge containerSpec.ExtraEnvs to override with merged values from StorefrontDeploymentContainer
	envs := util.MergeEnv(store.GetEnv(), containerSpec.ExtraEnvs)

	phpEnvs := GetStorefrontDeploymentCalculatedPHPFPMValues(store)

	envs = util.MergeEnv(envs, phpEnvs)

	memoryLimitMiB = int(store.Spec.StorefrontDeploymentContainer.Resources.Limits.Memory().Value() / (1024 * 1024))
	maxChildren = int(math.Round(float64(memoryLimitMiB / memoryPerChildMiB)))
	startServers = int(math.Round(float64(maxChildren) * startServersRatio))
	minSpare = int(math.Round(float64(maxChildren) * minSpareServersRatio))
	maxSpare = int(math.Round(float64(maxChildren)*maxSpareServersRatio) + 1)

	containers := append(containerSpec.ExtraContainers, corev1.Container{
		Name: DEPLOYMENT_STOREFRONT_CONTAINER_NAME,
		LivenessProbe: &corev1.Probe{
			ProbeHandler: corev1.ProbeHandler{
				HTTPGet: &corev1.HTTPGetAction{
					Path: "/api/_info/health-check",
					Port: intstr.IntOrString{
						Type:   intstr.Int,
						IntVal: containerSpec.Port,
					},
				},
			},
			TimeoutSeconds:      2,
			InitialDelaySeconds: 5,
		},
		ReadinessProbe: &corev1.Probe{
			ProbeHandler: corev1.ProbeHandler{
				HTTPGet: &corev1.HTTPGetAction{
					Path: "/api/_info/health-check",
					Port: intstr.IntOrString{
						Type:   intstr.Int,
						IntVal: containerSpec.Port,
					},
				},
			},
			TimeoutSeconds:      5,
			InitialDelaySeconds: 5,
		},
		Image:           containerSpec.Image,
		ImagePullPolicy: containerSpec.ImagePullPolicy,
		Env:             envs,
		VolumeMounts:    containerSpec.VolumeMounts,
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
			Name:        GetStorefrontDeploymentName(store),
			Namespace:   store.Namespace,
			Labels:      labels,
			Annotations: annotations,
		},
		Spec: appsv1.DeploymentSpec{
			ProgressDeadlineSeconds: &containerSpec.ProgressDeadlineSeconds,
			Replicas:                &containerSpec.Replicas,
			Selector: &metav1.LabelSelector{
				MatchLabels: util.GetStorefrontDeploymentMatchLabel(),
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
					SecurityContext:           containerSpec.SecurityContext,
					InitContainers:            containerSpec.InitContainers,
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

func GetStorefrontDeploymentName(store v1.Store) string {
	return fmt.Sprintf("%s-storefront", store.Name)
}

func GetStorefrontDeploymentCalculatedPHPFPMValues(store v1.Store) []corev1.EnvVar {
	memoryLimitMiB = int(store.Spec.StorefrontDeploymentContainer.Resources.Limits.Memory().Value() / (1024 * 1024))
	maxChildren = int(math.Round(float64(memoryLimitMiB / memoryPerChildMiB)))
	startServers = int(math.Round(float64(maxChildren) * startServersRatio))
	minSpare = int(math.Round(float64(maxChildren) * minSpareServersRatio))
	maxSpare = int(math.Round(float64(maxChildren)*maxSpareServersRatio) + 1)

	env := []corev1.EnvVar{
		{
			Name:  "PHP_FPM_PM",
			Value: "dynamic",
		},
		{
			Name:  "PHP_FPM_PM_MAX_CHILDREN",
			Value: fmt.Sprintf("%d", maxChildren),
		},
		{
			Name:  "PHP_FPM_PM_START_SERVERS",
			Value: fmt.Sprintf("%d", startServers),
		},
		{
			Name:  "PHP_FPM_PM_MIN_SPARE_SERVERS",
			Value: fmt.Sprintf("%d", minSpare),
		},
		{
			Name:  "PHP_FPM_PM_MAX_SPARE_SERVERS",
			Value: fmt.Sprintf("%d", maxSpare),
		},
	}
	return env
}

// The formulas are based on CPU cores and MiB, not Kubernetes base units.
