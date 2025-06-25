package deployment

import (
	"context"
	"fmt"
	"maps"

	v1 "github.com/shopware/shopware-operator/api/v1"
	"github.com/shopware/shopware-operator/internal/util"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/intstr"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

func GetAdminDeployment(
	ctx context.Context,
	store v1.Store,
	client client.Client,
) (*appsv1.Deployment, error) {
	setup := AdminDeployment(store)
	search := &appsv1.Deployment{
		ObjectMeta: setup.ObjectMeta,
	}
	err := client.Get(ctx, types.NamespacedName{
		Namespace: setup.Namespace,
		Name:      setup.Name,
	}, search)
	return search, err
}

func AdminDeployment(store v1.Store) *appsv1.Deployment {
	// Merge Overwritten adminContainer fields into container fields
	store.Spec.Container.Merge(store.Spec.AdminDeploymentContainer)

	appName := "shopware-admin"
	labels := util.GetDefaultContainerStoreLabels(store, store.Spec.AdminDeploymentContainer.Labels)
	maps.Copy(labels, util.GetAdminDeploymentMatchLabel())

	annotations := util.GetDefaultContainerAnnotations(appName, store, store.Spec.AdminDeploymentContainer.Annotations)

	containers := append(store.Spec.Container.ExtraContainers, corev1.Container{
		LivenessProbe: &corev1.Probe{
			ProbeHandler: corev1.ProbeHandler{
				HTTPGet: &corev1.HTTPGetAction{
					Path: "/api/_info/health-check",
					Port: intstr.IntOrString{
						Type:   intstr.Int,
						IntVal: store.Spec.Container.Port,
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
						IntVal: store.Spec.Container.Port,
					},
				},
			},
			TimeoutSeconds:      5,
			InitialDelaySeconds: 5,
		},
		Name:            appName,
		Image:           store.Spec.Container.Image,
		ImagePullPolicy: store.Spec.Container.ImagePullPolicy,
		Env:             store.GetEnv(),
		VolumeMounts:    store.Spec.Container.VolumeMounts,
		Ports: []corev1.ContainerPort{
			{
				ContainerPort: store.Spec.Container.Port,
				Protocol:      corev1.ProtocolTCP,
			},
		},
		Resources: store.Spec.Container.Resources,
	})

	deployment := &appsv1.Deployment{
		TypeMeta: metav1.TypeMeta{
			Kind:       "Deployment",
			APIVersion: "apps/v1",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:        GetAdminDeploymentName(store),
			Namespace:   store.Namespace,
			Labels:      labels,
			Annotations: annotations,
		},
		Spec: appsv1.DeploymentSpec{
			ProgressDeadlineSeconds: &store.Spec.Container.ProgressDeadlineSeconds,
			Replicas:                &store.Spec.Container.Replicas,

			Selector: &metav1.LabelSelector{
				MatchLabels: util.GetAdminDeploymentMatchLabel(),
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
					Volumes:                   store.Spec.Container.Volumes,
					TopologySpreadConstraints: store.Spec.Container.TopologySpreadConstraints,
					NodeSelector:              store.Spec.Container.NodeSelector,
					ImagePullSecrets:          store.Spec.Container.ImagePullSecrets,
					RestartPolicy:             store.Spec.Container.RestartPolicy,
					Containers:                containers,
					SecurityContext:           store.Spec.Container.SecurityContext,
				},
			},
		},
	}

	// Old way
	if store.Spec.ServiceAccountName != "" {
		deployment.Spec.Template.Spec.ServiceAccountName = store.Spec.ServiceAccountName
	}
	// New way
	if store.Spec.Container.ServiceAccountName != "" {
		deployment.Spec.Template.Spec.ServiceAccountName = store.Spec.Container.ServiceAccountName
	}

	return deployment
}

func GetAdminDeploymentName(store v1.Store) string {
	return fmt.Sprintf("%s-store-admin", store.Name)
}
