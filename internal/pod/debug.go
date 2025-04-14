package pod

import (
	"context"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"

	v1 "github.com/shopware/shopware-operator/api/v1"
	"github.com/shopware/shopware-operator/internal/deployment"
	"github.com/shopware/shopware-operator/internal/util"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

func GetDebugPod(ctx context.Context, client client.Client, store v1.Store, storeDebugInstance v1.StoreDebugInstance) (*corev1.Pod, error) {
	pod := DebugPod(store, storeDebugInstance)
	search := &corev1.Pod{
		ObjectMeta: pod.ObjectMeta,
	}
	err := client.Get(ctx, types.NamespacedName{
		Namespace: pod.Namespace,
		Name:      pod.Name,
	}, search)
	return search, err
}

func DebugPod(store v1.Store, storeDebugInstance v1.StoreDebugInstance) *corev1.Pod {
	podSpec := new(corev1.Pod)

	podSpec.ObjectMeta = metav1.ObjectMeta{
		Name:      storeDebugInstance.Name,
		Namespace: storeDebugInstance.Namespace,
	}

	store.Spec.Container.Merge(store.Spec.StorefrontDeploymentContainer)

	labels := util.GetDefaultStoreInstanceDebugLabels(store, storeDebugInstance)

	podSpec.Labels = labels
	podSpec.Spec.RestartPolicy = corev1.RestartPolicyNever

	ports := []corev1.ContainerPort{
		{
			ContainerPort: store.Spec.Container.Port,
			Protocol:      corev1.ProtocolTCP,
		},
	}
	ports = append(ports, storeDebugInstance.Spec.ExtraContainerPorts...)

	containers := append(store.Spec.Container.ExtraContainers, corev1.Container{
		Name: deployment.DEPLOYMENT_STOREFRONT_CONTAINER_NAME,
		// we don't need the liveness and readiness probe to make sure that the container always starts
		Image:           store.Spec.Container.Image,
		ImagePullPolicy: store.Spec.Container.ImagePullPolicy,
		Env:             store.GetEnv(),
		VolumeMounts:    store.Spec.Container.VolumeMounts,
		Ports:           ports,
		Resources:       store.Spec.Container.Resources,
	})

	podSpec.Spec.Containers = containers
	podSpec.Spec.Volumes = store.Spec.Container.Volumes
	podSpec.Spec.TopologySpreadConstraints = store.Spec.Container.TopologySpreadConstraints
	podSpec.Spec.NodeSelector = store.Spec.Container.NodeSelector
	podSpec.Spec.ImagePullSecrets = store.Spec.Container.ImagePullSecrets
	podSpec.Spec.SecurityContext = store.Spec.Container.SecurityContext

	if store.Spec.ServiceAccountName != "" {
		podSpec.Spec.ServiceAccountName = store.Spec.ServiceAccountName
	}
	if store.Spec.Container.ServiceAccountName != "" {
		podSpec.Spec.ServiceAccountName = store.Spec.Container.ServiceAccountName
	}

	return podSpec
}
