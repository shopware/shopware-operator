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
	return &policy.PodDisruptionBudget{
		TypeMeta: metav1.TypeMeta{},
		ObjectMeta: metav1.ObjectMeta{
			Name:      GetStorefrontPDBName(store),
			Namespace: store.GetNamespace(),
			Labels:    util.GetDefaultStoreLabels(store),
		},
		Spec: policy.PodDisruptionBudgetSpec{
			MaxUnavailable: &intstr.IntOrString{
				IntVal: 1,
				Type:   intstr.Int,
			},
			Selector: &metav1.LabelSelector{
				MatchLabels: util.GetStorefrontDeploymentMatchLabel(nil),
			},
		},
	}
}

func WorkerPDB(store v1.Store) *policy.PodDisruptionBudget {
	return &policy.PodDisruptionBudget{
		TypeMeta: metav1.TypeMeta{},
		ObjectMeta: metav1.ObjectMeta{
			Name:      GetWorkerPDBName(store),
			Namespace: store.GetNamespace(),
			Labels:    util.GetDefaultStoreLabels(store),
		},
		Spec: policy.PodDisruptionBudgetSpec{
			MaxUnavailable: &intstr.IntOrString{
				IntVal: 1,
				Type:   intstr.Int,
			},
			Selector: &metav1.LabelSelector{
				MatchLabels: util.GetWorkerDeploymentMatchLabel(nil),
			},
		},
	}
}

func AdminPDB(store v1.Store) *policy.PodDisruptionBudget {
	return &policy.PodDisruptionBudget{
		TypeMeta: metav1.TypeMeta{},
		ObjectMeta: metav1.ObjectMeta{
			Name:      GetAdminPDBName(store),
			Namespace: store.GetNamespace(),
			Labels:    util.GetDefaultStoreLabels(store),
		},
		Spec: policy.PodDisruptionBudgetSpec{
			MaxUnavailable: &intstr.IntOrString{
				IntVal: 1,
				Type:   intstr.Int,
			},
			Selector: &metav1.LabelSelector{
				MatchLabels: util.GetAdminDeploymentMatchLabel(nil),
			},
		},
	}
}

func GetStorefrontPDBName(store v1.Store) string {
	return fmt.Sprintf("storefront-%s", store.Name)
}

func GetAdminPDBName(store v1.Store) string {
	return fmt.Sprintf("admin-%s", store.Name)
}

func GetWorkerPDBName(store v1.Store) string {
	return fmt.Sprintf("worker-%s", store.Name)
}
