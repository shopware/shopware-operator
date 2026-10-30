package controller

import (
	"testing"

	v1 "github.com/shopware/shopware-operator/api/v1"
	"github.com/stretchr/testify/require"
	appsv1 "k8s.io/api/apps/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/utils/ptr"
	"sigs.k8s.io/controller-runtime/pkg/event"
	"sigs.k8s.io/controller-runtime/pkg/predicate"
)

func ownedDeployment(name string) *appsv1.Deployment {
	return &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: "shop",
			OwnerReferences: []metav1.OwnerReference{{
				APIVersion: "shop.shopware.com/v1",
				Kind:       "Store",
				Name:       "store",
				Controller: ptr.To(true),
			}},
		},
	}
}

func TestReconcileReasons(t *testing.T) {
	storeKey := types.NamespacedName{Namespace: "shop", Name: "store"}

	t.Run("no recorded reason means requeue", func(t *testing.T) {
		var reasons reconcileReasons
		require.Equal(t, []string{requeueReconcileReason}, reasons.pop(storeKey))
	})

	t.Run("records reasons for the owning store and clears them on pop", func(t *testing.T) {
		var reasons reconcileReasons
		p := reasons.wrap("hash annotation changed", predicate.Funcs{})

		deployment := ownedDeployment("store-storefront")
		require.True(t, p.Update(event.UpdateEvent{ObjectOld: deployment, ObjectNew: deployment}))
		require.True(t, p.Update(event.UpdateEvent{ObjectOld: deployment, ObjectNew: deployment}))
		require.True(t, p.Delete(event.DeleteEvent{Object: ownedDeployment("store-admin")}))

		require.Equal(t, []string{
			"hash annotation changed: update Deployment/store-storefront",
			"hash annotation changed: delete Deployment/store-admin",
		}, reasons.pop(storeKey))
		require.Equal(t, []string{requeueReconcileReason}, reasons.pop(storeKey))
	})

	t.Run("filtered events record nothing", func(t *testing.T) {
		var reasons reconcileReasons
		p := reasons.wrap("hash annotation changed", predicate.Funcs{
			UpdateFunc: func(event.UpdateEvent) bool { return false },
		})

		deployment := ownedDeployment("store-storefront")
		require.False(t, p.Update(event.UpdateEvent{ObjectOld: deployment, ObjectNew: deployment}))
		require.Equal(t, []string{requeueReconcileReason}, reasons.pop(storeKey))
	})

	t.Run("store events are recorded for the store itself", func(t *testing.T) {
		var reasons reconcileReasons
		p := reasons.wrap("store spec changed", predicate.Funcs{})

		store := &v1.Store{ObjectMeta: metav1.ObjectMeta{Name: "store", Namespace: "shop"}}
		require.True(t, p.Create(event.CreateEvent{Object: store}))
		require.Equal(t, []string{"store spec changed: create Store/store"}, reasons.pop(storeKey))
	})

	t.Run("objects without store owner are ignored", func(t *testing.T) {
		var reasons reconcileReasons
		p := reasons.wrap("hash annotation changed", predicate.Funcs{})

		deployment := &appsv1.Deployment{ObjectMeta: metav1.ObjectMeta{Name: "other", Namespace: "shop"}}
		require.True(t, p.Create(event.CreateEvent{Object: deployment}))
		require.Equal(t, []string{requeueReconcileReason}, reasons.pop(storeKey))
	})

	t.Run("caps the number of reasons", func(t *testing.T) {
		var reasons reconcileReasons
		for i := range maxReconcileReasons + 5 {
			reasons.add(storeKey, string(rune('a'+i)))
		}
		require.Len(t, reasons.pop(storeKey), maxReconcileReasons)
	})
}
