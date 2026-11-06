package pdb

import (
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
			MatchLabels: util.GetStorefrontDeploymentMatchLabel(),
		},
		MaxUnavailable: &intstr.IntOrString{
			IntVal: 1,
			Type:   intstr.Int,
		},
	}

	return &policy.PodDisruptionBudget{
		TypeMeta: metav1.TypeMeta{},
		ObjectMeta: metav1.ObjectMeta{
			Name:      util.GetStorefrontPDBName(store),
			Namespace: store.GetNamespace(),
			Labels:    util.GetStoreLabel(store),
		},
		Spec: spec,
	}
}

func WorkerPDB(store v1.Store) *policy.PodDisruptionBudget {
	store.Spec.Container.Merge(store.Spec.WorkerDeploymentContainer)

	spec := policy.PodDisruptionBudgetSpec{
		Selector: &metav1.LabelSelector{
			MatchLabels: util.GetWorkerDeploymentMatchLabel(),
		},
		MaxUnavailable: &intstr.IntOrString{
			IntVal: 1,
			Type:   intstr.Int,
		},
	}

	return &policy.PodDisruptionBudget{
		TypeMeta: metav1.TypeMeta{},
		ObjectMeta: metav1.ObjectMeta{
			Name:      util.GetWorkerPDBName(store),
			Namespace: store.GetNamespace(),
			Labels:    util.GetStoreLabel(store),
		},
		Spec: spec,
	}
}

func AdminPDB(store v1.Store) *policy.PodDisruptionBudget {
	store.Spec.Container.Merge(store.Spec.AdminDeploymentContainer)

	spec := policy.PodDisruptionBudgetSpec{
		Selector: &metav1.LabelSelector{
			MatchLabels: util.GetAdminDeploymentMatchLabel(),
		},
		MaxUnavailable: &intstr.IntOrString{
			IntVal: 1,
			Type:   intstr.Int,
		},
	}

	return &policy.PodDisruptionBudget{
		TypeMeta: metav1.TypeMeta{},
		ObjectMeta: metav1.ObjectMeta{
			Name:      util.GetAdminPDBName(store),
			Namespace: store.GetNamespace(),
			Labels:    util.GetStoreLabel(store),
		},
		Spec: spec,
	}
}
