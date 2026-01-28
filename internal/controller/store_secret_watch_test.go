package controller

import (
	"context"
	"testing"

	v1 "github.com/shopware/shopware-operator/api/v1"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

func TestFindStoreForReconcileOnlyEnqueuesReferencingStores(t *testing.T) {
	scheme := runtime.NewScheme()
	require.NoError(t, v1.AddToScheme(scheme))
	require.NoError(t, corev1.AddToScheme(scheme))

	storeA := &v1.Store{
		ObjectMeta: metav1.ObjectMeta{Name: "store-a", Namespace: "shop"},
		Spec: v1.StoreSpec{
			SecretName: "store-a-secret",
			S3Storage:  v1.S3Storage{AccessKeyRef: v1.SecretRef{Name: "shared-s3", Key: "access"}},
		},
	}
	storeB := &v1.Store{
		ObjectMeta: metav1.ObjectMeta{Name: "store-b", Namespace: "shop"},
		Spec: v1.StoreSpec{
			SecretName: "store-b-secret",
			S3Storage:  v1.S3Storage{AccessKeyRef: v1.SecretRef{Name: "shared-s3", Key: "access"}},
		},
	}
	otherNamespace := &v1.Store{
		ObjectMeta: metav1.ObjectMeta{Name: "store-c", Namespace: "other"},
		Spec:       v1.StoreSpec{SecretName: "store-a-secret"},
	}

	r := &StoreReconciler{
		Client: fake.NewClientBuilder().WithScheme(scheme).WithObjects(storeA, storeB, otherNamespace).Build(),
		Logger: zap.NewNop().Sugar(),
	}

	requestNames := func(secretName string) []string {
		secret := &corev1.Secret{ObjectMeta: metav1.ObjectMeta{Name: secretName, Namespace: "shop"}}
		requests := r.findStoreForReconcile(context.Background(), secret)
		names := make([]string, 0, len(requests))
		for _, req := range requests {
			names = append(names, req.Name)
		}
		return names
	}

	require.Equal(t, []string{"store-a"}, requestNames("store-a-secret"))
	require.ElementsMatch(t, []string{"store-a", "store-b"}, requestNames("shared-s3"))
	require.Empty(t, requestNames("unrelated"))
	require.Equal(t,
		[]string{"secret changed: Secret/store-a-secret", "secret changed: Secret/shared-s3"},
		r.reasons.pop(types.NamespacedName{Namespace: "shop", Name: "store-a"}),
	)
}
