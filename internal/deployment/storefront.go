package deployment

import (
	"context"
	"fmt"
	"maps"

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

const DEPLOYMENT_STOREFRONT_CONTAINER_NAME = "shopware-storefront"
const FPM_ADMIN_PORT int32 = 8001

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
	containerSpec.Volumes = append(containerSpec.Volumes, store.GetDatabaseTLSVolumes()...)
	containerSpec.VolumeMounts = append(containerSpec.VolumeMounts, store.GetDatabaseTLSVolumeMounts()...)

	// think of the debug container to when changing the deployment

	appName := "shopware-storefront"
	labels := util.GetDefaultContainerStoreLabels(store, store.Spec.StorefrontDeploymentContainer.Labels)
	maps.Copy(labels, util.GetStorefrontDeploymentMatchLabel())

	annotations := util.GetDefaultContainerAnnotations(appName, store, store.Spec.StorefrontDeploymentContainer.Annotations)

	envs := util.MergeEnv(store.GetEnv(), containerSpec.ExtraEnvs)
	if store.Spec.FPM.ProcessManagement == "operator" {
		if containerSpec.Resources.Limits.Memory() != nil && containerSpec.Resources.Limits.Memory().Value() != 0 {
			phpEnvs := GetCalculatedPHPFPMValues(int(containerSpec.Resources.Limits.Memory().Value() / (1024 * 1024)))
			envs = util.MergeEnv(envs, phpEnvs)
		} else {
			phpEnvs := GetCalculatedPHPFPMValues(2048)
			envs = util.MergeEnv(envs, phpEnvs)
			fmt.Println("envs: ", phpEnvs)
		}
	} else if store.Spec.FPM.ProcessManagement == "frankenphp" {
		if containerSpec.Resources.Limits.Memory() != nil && containerSpec.Resources.Limits.Memory().Value() != 0 {
			phpEnvs := GetCalculatedFrankenPHPValues(int(containerSpec.Resources.Limits.Memory().Value() / (1024 * 1024)))
			envs = util.MergeEnv(envs, phpEnvs)
		} else {
			phpEnvs := GetCalculatedFrankenPHPValues(2048)
			envs = util.MergeEnv(envs, phpEnvs)
		}
	}

	containers := append(util.DefaultContainerSecurityContexts(containerSpec.ExtraContainers), corev1.Container{
		Name:            DEPLOYMENT_STOREFRONT_CONTAINER_NAME,
		StartupProbe:    storefrontStartupProbe(store),
		LivenessProbe:   storefrontLivenessProbe(store),
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
		SecurityContext: util.RestrictedContainerSecurityContext(),
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

func GetStorefrontDeploymentName(store v1.Store) string {
	return fmt.Sprintf("%s-storefront", store.Name)
}

// storefrontStartupProbe returns the startup probe based on the runtime mode.
// FrankenPHP uses the Caddy admin API on port 2019, PHP-FPM uses the FPM ping on port 8001.
func storefrontStartupProbe(store v1.Store) *corev1.Probe {
	if store.Spec.FPM.ProcessManagement == "frankenphp" {
		return &corev1.Probe{
			ProbeHandler: corev1.ProbeHandler{
				HTTPGet: &corev1.HTTPGetAction{
					Path: "/config/",
					Port: intstr.IntOrString{
						Type:   intstr.Int,
						IntVal: 2019,
					},
				},
			},
			PeriodSeconds:    5,
			TimeoutSeconds:   5,
			FailureThreshold: 18,
		}
	}
	return &corev1.Probe{
		ProbeHandler: corev1.ProbeHandler{
			HTTPGet: &corev1.HTTPGetAction{
				Path: "/-/fpm/ping",
				Port: intstr.IntOrString{
					Type:   intstr.Int,
					IntVal: FPM_ADMIN_PORT,
				},
			},
		},
		PeriodSeconds:    5,
		TimeoutSeconds:   5,
		FailureThreshold: 18,
	}
}

// storefrontLivenessProbe returns the liveness probe based on the runtime mode.
func storefrontLivenessProbe(store v1.Store) *corev1.Probe {
	if store.Spec.FPM.ProcessManagement == "frankenphp" {
		return &corev1.Probe{
			ProbeHandler: corev1.ProbeHandler{
				HTTPGet: &corev1.HTTPGetAction{
					Path: "/config/",
					Port: intstr.IntOrString{
						Type:   intstr.Int,
						IntVal: 2019,
					},
				},
			},
			PeriodSeconds:    10,
			TimeoutSeconds:   5,
			FailureThreshold: 3,
		}
	}
	return &corev1.Probe{
		ProbeHandler: corev1.ProbeHandler{
			HTTPGet: &corev1.HTTPGetAction{
				Path: "/-/fpm/ping",
				Port: intstr.IntOrString{
					Type:   intstr.Int,
					IntVal: FPM_ADMIN_PORT,
				},
			},
		},
		PeriodSeconds:    10,
		TimeoutSeconds:   5,
		FailureThreshold: 3,
	}
}
