package deployment

import (
	"context"
	"fmt"

	v1 "github.com/shopware/shopware-operator/api/v1"
	"github.com/shopware/shopware-operator/internal/util"
	"golang.org/x/exp/maps"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/intstr"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

func GetWorkerDeployment(
	ctx context.Context,
	store *v1.Store,
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

func WorkerDeployment(store *v1.Store) *appsv1.Deployment {
	appName := "shopware-worker"
	labels := map[string]string{
		"app": appName,
	}
	maps.Copy(labels, util.GetDefaultLabels(store))

	containers := append(store.Spec.Container.ExtraContainers, corev1.Container{
		Name:            appName,
		Image:           store.Spec.Container.Image,
		ImagePullPolicy: store.Spec.Container.ImagePullPolicy,
		Env:             store.GetEnv(),
		Command: []string{
			"bin/console",
		},
		Args: []string{
			"messenger:consume",
			"async",
			"low_priority",
			"failed",
			"scheduler_shopware",
		},
		VolumeMounts: store.Spec.Container.VolumeMounts,
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
			Name:        GetWorkerDeploymentName(store),
			Namespace:   store.Namespace,
			Labels:      labels,
			Annotations: store.Spec.Container.Annotations,
		},
		Spec: appsv1.DeploymentSpec{
			ProgressDeadlineSeconds: &store.Spec.Container.ProgressDeadlineSeconds,
			Replicas:                &store.Spec.Container.Replicas,
			Selector: &metav1.LabelSelector{
				MatchLabels: map[string]string{
					"app": appName,
				},
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
					Labels: labels,
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

	if store.Spec.ServiceAccountName != "" {
		deployment.Spec.Template.Spec.ServiceAccountName = store.Spec.ServiceAccountName
	}

	return deployment
}

func GetWorkerDeploymentName(store *v1.Store) string {
	return fmt.Sprintf("%s-store-worker", store.Name)
}
