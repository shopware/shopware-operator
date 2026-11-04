package controller

import (
	"context"
	"errors"
	"fmt"
	"testing"

	shopv1 "github.com/shopware/shopware-operator/api/v1"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/tools/record"
	"k8s.io/utils/ptr"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/client/interceptor"
)

func terminalDebugTestScheme(t *testing.T) *runtime.Scheme {
	t.Helper()

	scheme := runtime.NewScheme()
	require.NoError(t, clientgoscheme.AddToScheme(scheme))
	require.NoError(t, shopv1.AddToScheme(scheme))

	return scheme
}

func terminalDebugTestInstance() *shopv1.TerminalDebugInstance {
	return &shopv1.TerminalDebugInstance{
		ObjectMeta: metav1.ObjectMeta{Name: "app", Namespace: "default", UID: "instance-uid"},
		Spec: shopv1.TerminalDebugInstanceSpec{
			PodSelector: metav1.LabelSelector{MatchLabels: map[string]string{"app": "app"}},
		},
	}
}

// unrelatedPod is an application pod that happens to share the instance name.
func unrelatedPod() *corev1.Pod {
	return &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{Name: "app", Namespace: "default", UID: "app-uid", Labels: map[string]string{"app": "app"}},
		Status:     corev1.PodStatus{Phase: corev1.PodRunning},
	}
}

func TestTerminalDebugPodNameCollisionIsRejectedAndNotDeleted(t *testing.T) {
	ctx := context.Background()
	scheme := terminalDebugTestScheme(t)
	instance := terminalDebugTestInstance()

	cl := fake.NewClientBuilder().WithScheme(scheme).WithObjects(unrelatedPod()).Build()
	r := &TerminalDebugInstanceReconciler{Client: cl, Scheme: scheme, Recorder: record.NewFakeRecorder(10)}

	status := r.reconcileDebugPod(ctx, instance)
	assert.Equal(t, shopv1.StoreDebugInstanceStateError, status.State)
	assert.Empty(t, status.PodName)
	assert.Empty(t, status.Container)

	// The error state triggers cleanup, which must leave the unrelated pod alone.
	instance.Status = status
	_, err := r.cleanup(ctx, instance)
	require.NoError(t, err)

	got := &corev1.Pod{}
	require.NoError(t, cl.Get(ctx, types.NamespacedName{Namespace: "default", Name: "app"}, got))
	assert.Equal(t, types.UID("app-uid"), got.UID)
}

func controlledDebugPod(instance *shopv1.TerminalDebugInstance, containers ...string) *corev1.Pod {
	p := unrelatedPod()
	p.OwnerReferences = []metav1.OwnerReference{{
		APIVersion: shopv1.GroupVersion.String(), Kind: "TerminalDebugInstance",
		Name: instance.Name, UID: instance.UID, Controller: ptr.To(true),
	}}
	for _, name := range containers {
		p.Spec.Containers = append(p.Spec.Containers, corev1.Container{Name: name})
	}
	return p
}

func TestTerminalDebugExistingPodRecoversContainer(t *testing.T) {
	tests := []struct {
		name          string
		requested     string
		containers    []string
		prior         string
		wantContainer string
		wantState     shopv1.StoreDebugInstanceState
	}{
		{name: "sole container", containers: []string{"shop"}, wantContainer: "shop", wantState: shopv1.StoreDebugInstanceStateRunning},
		{name: "explicit container", requested: "sidecar", containers: []string{"shop", "sidecar"}, wantContainer: "sidecar", wantState: shopv1.StoreDebugInstanceStateRunning},
		{name: "recorded container is kept", requested: "other", containers: []string{"shop"}, prior: "shop", wantContainer: "shop", wantState: shopv1.StoreDebugInstanceStateRunning},
		{name: "ambiguous container", containers: []string{"shop", "sidecar"}, wantState: shopv1.StoreDebugInstanceStateError},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx := context.Background()
			scheme := terminalDebugTestScheme(t)
			instance := terminalDebugTestInstance()
			instance.Spec.Container = tt.requested
			instance.Status.Container = tt.prior

			cl := fake.NewClientBuilder().WithScheme(scheme).WithObjects(controlledDebugPod(instance, tt.containers...)).Build()
			r := &TerminalDebugInstanceReconciler{Client: cl, Scheme: scheme, Recorder: record.NewFakeRecorder(10)}

			status := r.reconcileDebugPod(ctx, instance)
			assert.Equal(t, tt.wantState, status.State)
			assert.Equal(t, tt.wantContainer, status.Container)
		})
	}
}

func TestTerminalDebugBuildFailureClassification(t *testing.T) {
	listFailure := func(kind string) interceptor.Funcs {
		return interceptor.Funcs{
			List: func(ctx context.Context, c client.WithWatch, list client.ObjectList, opts ...client.ListOption) error {
				if fmt.Sprintf("%T", list) == kind {
					return errors.New("timeout")
				}
				return c.List(ctx, list, opts...)
			},
		}
	}

	source := unrelatedPod()
	source.Name = "source"
	source.Spec.Containers = []corev1.Container{{Name: "shop"}}

	tests := []struct {
		name       string
		funcs      interceptor.Funcs
		selector   *metav1.LabelSelector
		container  string
		wantState  shopv1.StoreDebugInstanceState
		wantReason string
	}{
		{name: "pod list timeout", funcs: listFailure("*v1.PodList"), wantState: shopv1.StoreDebugInstanceStatePending, wantReason: "ReconcileError"},
		{name: "replicaset list timeout", funcs: listFailure("*v1.ReplicaSetList"), wantState: shopv1.StoreDebugInstanceStatePending, wantReason: "ReconcileError"},
		{name: "service list timeout", funcs: listFailure("*v1.ServiceList"), wantState: shopv1.StoreDebugInstanceStatePending, wantReason: "ReconcileError"},
		{name: "empty selector", selector: &metav1.LabelSelector{}, wantState: shopv1.StoreDebugInstanceStateError, wantReason: "InvalidSource"},
		{name: "unknown container", container: "missing", wantState: shopv1.StoreDebugInstanceStateError, wantReason: "InvalidSource"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			scheme := terminalDebugTestScheme(t)
			instance := terminalDebugTestInstance()
			instance.Name = "debug"
			instance.Spec.Container = tt.container
			if tt.selector != nil {
				instance.Spec.PodSelector = *tt.selector
			}

			cl := fake.NewClientBuilder().WithScheme(scheme).WithObjects(source).WithInterceptorFuncs(tt.funcs).Build()
			r := &TerminalDebugInstanceReconciler{Client: cl, Scheme: scheme, Recorder: record.NewFakeRecorder(10)}

			status := r.reconcileDebugPod(context.Background(), instance)
			assert.Equal(t, tt.wantState, status.State)
			require.NotEmpty(t, status.Conditions)
			assert.Equal(t, tt.wantReason, status.Conditions[len(status.Conditions)-1].Reason)
		})
	}
}

func TestTerminalDebugCleanupDeletesControlledPod(t *testing.T) {
	ctx := context.Background()
	scheme := terminalDebugTestScheme(t)
	instance := terminalDebugTestInstance()

	owned := unrelatedPod()
	owned.OwnerReferences = []metav1.OwnerReference{{
		APIVersion: shopv1.GroupVersion.String(), Kind: "TerminalDebugInstance",
		Name: instance.Name, UID: instance.UID, Controller: ptr.To(true),
	}}

	cl := fake.NewClientBuilder().WithScheme(scheme).WithObjects(owned).Build()
	r := &TerminalDebugInstanceReconciler{Client: cl, Scheme: scheme, Recorder: record.NewFakeRecorder(10)}

	_, err := r.cleanup(ctx, instance)
	require.NoError(t, err)

	got := &corev1.Pod{}
	err = cl.Get(ctx, types.NamespacedName{Namespace: "default", Name: "app"}, got)
	assert.Error(t, err)
}

func TestTerminalDebugCleanupKeepsForeignService(t *testing.T) {
	ctx := context.Background()
	scheme := terminalDebugTestScheme(t)
	instance := terminalDebugTestInstance()

	svc := &corev1.Service{ObjectMeta: metav1.ObjectMeta{Name: "app", Namespace: "default", UID: "svc-uid"}}
	cl := fake.NewClientBuilder().WithScheme(scheme).WithObjects(svc).Build()
	r := &TerminalDebugInstanceReconciler{Client: cl, Scheme: scheme, Recorder: record.NewFakeRecorder(10)}

	_, err := r.cleanup(ctx, instance)
	require.NoError(t, err)

	got := &corev1.Service{}
	require.NoError(t, cl.Get(ctx, types.NamespacedName{Namespace: "default", Name: "app"}, got))
}
