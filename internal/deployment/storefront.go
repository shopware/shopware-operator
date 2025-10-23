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
				//nolint:staticcheck
				Message: fmt.Errorf("Error on client get: %w", err).Error(),
				Ready:   "0/0",
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
