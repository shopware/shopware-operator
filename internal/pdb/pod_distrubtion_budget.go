package pdb

import (
	"fmt"

	v1 "github.com/shopware/shopware-operator/api/v1"
	"github.com/shopware/shopware-operator/internal/util"
	policy "k8s.io/api/policy/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
)

func StorefrontPDB(store v1.Store) *policy.PodDisruptionBudget {
	store.Spec.Container.Merge(store.Spec.StorefrontDeploymentContainer)

	spec := policy.PodDisruptionBudgetSpec{
		Selector: &metav1.LabelSelector{
			MatchLabels: util.GetStorefrontDeploymentMatchLabel(nil),
		},
		MaxUnavailable: &intstr.IntOrString{
			IntVal: 1,
			Type:   intstr.Int,
		},
	}

	return &policy.PodDisruptionBudget{
		TypeMeta: metav1.TypeMeta{},
		ObjectMeta: metav1.ObjectMeta{
			Name:      GetStorefrontPDBName(store),
			Namespace: store.GetNamespace(),
			Labels:    util.GetDefaultStoreLabels(store),
		},
		Spec: spec,
	}
}

func WorkerPDB(store v1.Store) *policy.PodDisruptionBudget {
	store.Spec.Container.Merge(store.Spec.WorkerDeploymentContainer)

	spec := policy.PodDisruptionBudgetSpec{
		Selector: &metav1.LabelSelector{
			MatchLabels: util.GetWorkerDeploymentMatchLabel(nil),
		},
		MaxUnavailable: &intstr.IntOrString{
			IntVal: 1,
			Type:   intstr.Int,
		},
	}

	return &policy.PodDisruptionBudget{
		TypeMeta: metav1.TypeMeta{},
		ObjectMeta: metav1.ObjectMeta{
			Name:      GetWorkerPDBName(store),
			Namespace: store.GetNamespace(),
			Labels:    util.GetDefaultStoreLabels(store),
		},
		Spec: spec,
	}
}

func AdminPDB(store v1.Store) *policy.PodDisruptionBudget {
	store.Spec.Container.Merge(store.Spec.AdminDeploymentContainer)

	spec := policy.PodDisruptionBudgetSpec{
		Selector: &metav1.LabelSelector{
			MatchLabels: util.GetAdminDeploymentMatchLabel(nil),
		},
		MaxUnavailable: &intstr.IntOrString{
			IntVal: 1,
			Type:   intstr.Int,
		},
	}

	return &policy.PodDisruptionBudget{
		TypeMeta: metav1.TypeMeta{},
		ObjectMeta: metav1.ObjectMeta{
			Name:      GetAdminPDBName(store),
			Namespace: store.GetNamespace(),
			Labels:    util.GetDefaultStoreLabels(store),
		},
		Spec: spec,
	}
}

func GetStorefrontPDBName(store v1.Store) string {
	return fmt.Sprintf("%s-storefront", store.Name)
}

func GetAdminPDBName(store v1.Store) string {
	return fmt.Sprintf("%s-admin", store.Name)
}

func GetWorkerPDBName(store v1.Store) string {
	return fmt.Sprintf("%s-worker", store.Name)
}
