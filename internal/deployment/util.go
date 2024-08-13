package deployment

import (
	"context"

	v1 "github.com/shopware/shopware-operator/api/v1"
	appsv1 "k8s.io/api/apps/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

func GetStoreDeploymentImage(ctx context.Context, store *v1.Store, client client.Client) (string, error) {
	setup := StorefrontDeployment(store)
	search := &appsv1.Deployment{
		ObjectMeta: setup.ObjectMeta,
	}
	err := client.Get(ctx, types.NamespacedName{
		Namespace: setup.Namespace,
		Name:      setup.Name,
	}, search)
	if err != nil && errors.IsNotFound(err) {
		return "", nil
	}
	if err != nil {
		return "", err
	}

	return search.Spec.Template.Spec.Containers[0].Image, err
}