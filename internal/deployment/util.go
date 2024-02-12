package deployment

import (
	"context"
	"fmt"

	v1 "github.com/shopware/shopware-operator/api/v1"
	appsv1 "k8s.io/api/apps/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

func GetStoreDeploymentImage(
	ctx context.Context,
	store *v1.Store,
	client client.Client,
) (string, error) {
	setup := StorefrontDeployment(store)
	search := &appsv1.Deployment{
		ObjectMeta: setup.ObjectMeta,
	}
	err := client.Get(ctx, types.NamespacedName{
		Namespace: setup.Namespace,
		Name:      setup.Name,
	}, search)
	if err != nil {
		return "", err
	}

	for _, container := range search.Spec.Template.Spec.Containers {
		if container.Name == GetStorefrontDeploymentName(store) {
			return container.Image, nil
		}
	}
	return "", fmt.Errorf("Could'n find storefront deployment container")
}
