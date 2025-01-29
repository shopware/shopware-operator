package pdb

import (
	"context"
	"fmt"

	v1 "github.com/shopware/shopware-operator/api/v1"
	"github.com/shopware/shopware-operator/internal/util"
	policy "k8s.io/api/policy/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/intstr"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

func GetStorePDB(
	ctx context.Context,
	store v1.Store,
	client client.Client,
) (*policy.PodDisruptionBudget, error) {
	pdb := StorePDB(store)
	search := &policy.PodDisruptionBudget{
		ObjectMeta: pdb.ObjectMeta,
	}
	err := client.Get(ctx, types.NamespacedName{
		Namespace: pdb.Namespace,
		Name:      pdb.Name,
	}, search)
	return search, err
}

func StorePDB(store v1.Store) *policy.PodDisruptionBudget {
	return &policy.PodDisruptionBudget{
		TypeMeta: metav1.TypeMeta{},
		ObjectMeta: metav1.ObjectMeta{
			Name:        GetStorePDBName(store),
			Namespace:   store.GetNamespace(),
			Labels:      util.GetDefaultStoreLabels(store),
			Annotations: store.Spec.HorizontalPodAutoscaler.Annotations,
		},
		Spec: policy.PodDisruptionBudgetSpec{
			MinAvailable: &intstr.IntOrString{
				IntVal: 1,
				Type:   intstr.Int,
			},
			Selector: &metav1.LabelSelector{
				MatchLabels: util.GetPDBLabels(store),
			},
		},
	}
}

func GetStorePDBName(store v1.Store) string {
	return fmt.Sprintf("store-%s", store.Name)
}
