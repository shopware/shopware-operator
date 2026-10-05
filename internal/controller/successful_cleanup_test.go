package controller

import (
	"context"
	"testing"
	"time"

	shopv1 "github.com/shopware/shopware-operator/api/v1"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

const cleanupTestNamespace = "default"

func TestStoreExecCleanupDeletesFinishedExecAfterCleanupPeriod(t *testing.T) {
	ctx := context.Background()
	scheme := cleanupTestScheme(t)

	tests := []struct {
		name         string
		state        shopv1.StatefulState
		successfulIn time.Duration
		errorIn      time.Duration
	}{
		{name: "done", state: shopv1.ExecStateDone, successfulIn: time.Hour},
		{name: "error", state: shopv1.ExecStateError, errorIn: time.Hour},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ex := storeExecForCleanup(tt.name+"-old", tt.state, time.Now().Add(-2*time.Hour))
			ex.Spec.CleanupPeriodSuccessfulExec = &metav1.Duration{Duration: tt.successfulIn}
			ex.Spec.CleanupPeriodErrorExec = &metav1.Duration{Duration: tt.errorIn}

			cl := fake.NewClientBuilder().
				WithScheme(scheme).
				WithObjects(ex).
				Build()

			reconciler := StoreExecReconciler{Client: cl}

			result, handled, err := reconciler.reconcileStoreExecCleanup(ctx, ex)

			require.NoError(t, err)
			assert.True(t, handled)
			assert.Zero(t, result)

			got := &shopv1.StoreExec{}
			err = cl.Get(ctx, types.NamespacedName{Namespace: cleanupTestNamespace, Name: ex.Name}, got)
			assert.True(t, k8serrors.IsNotFound(err))
		})
	}
}

func TestStoreExecCleanupUsesDefaultPeriodsWhenUnset(t *testing.T) {
	ctx := context.Background()
	scheme := cleanupTestScheme(t)

	tests := []struct {
		name       string
		state      shopv1.StatefulState
		finishedAt time.Time
		wantGone   bool
	}{
		{name: "done past default", state: shopv1.ExecStateDone, finishedAt: time.Now().Add(-10 * time.Minute), wantGone: true},
		{name: "done within default", state: shopv1.ExecStateDone, finishedAt: time.Now().Add(-time.Minute)},
		{name: "error past default", state: shopv1.ExecStateError, finishedAt: time.Now().Add(-2 * time.Hour), wantGone: true},
		{name: "error within default", state: shopv1.ExecStateError, finishedAt: time.Now().Add(-10 * time.Minute)},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Both periods stay nil: an object whose defaults never ran must still
			// be cleaned up rather than silently retained forever.
			ex := storeExecForCleanup(tt.name, tt.state, tt.finishedAt)

			cl := fake.NewClientBuilder().
				WithScheme(scheme).
				WithObjects(ex).
				Build()

			reconciler := StoreExecReconciler{Client: cl}

			_, handled, err := reconciler.reconcileStoreExecCleanup(ctx, ex)

			require.NoError(t, err)
			assert.True(t, handled)

			got := &shopv1.StoreExec{}
			err = cl.Get(ctx, types.NamespacedName{Namespace: cleanupTestNamespace, Name: ex.Name}, got)
			assert.Equal(t, tt.wantGone, k8serrors.IsNotFound(err))
		})
	}
}

func TestStoreExecCleanupRetainsFinishedExecBeforeCleanupPeriod(t *testing.T) {
	ctx := context.Background()
	scheme := cleanupTestScheme(t)

	tests := []struct {
		name         string
		state        shopv1.StatefulState
		successfulIn time.Duration
		errorIn      time.Duration
	}{
		{name: "done", state: shopv1.ExecStateDone, successfulIn: time.Hour},
		{name: "error", state: shopv1.ExecStateError, errorIn: time.Hour},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ex := storeExecForCleanup(tt.name+"-new", tt.state, time.Now().Add(-30*time.Minute))
			ex.Spec.CleanupPeriodSuccessfulExec = &metav1.Duration{Duration: tt.successfulIn}
			ex.Spec.CleanupPeriodErrorExec = &metav1.Duration{Duration: tt.errorIn}

			cl := fake.NewClientBuilder().
				WithScheme(scheme).
				WithObjects(ex).
				Build()

			reconciler := StoreExecReconciler{Client: cl}

			result, handled, err := reconciler.reconcileStoreExecCleanup(ctx, ex)

			require.NoError(t, err)
			assert.True(t, handled)
			assert.Greater(t, result.RequeueAfter, time.Duration(0))

			got := &shopv1.StoreExec{}
			require.NoError(t, cl.Get(ctx, types.NamespacedName{Namespace: cleanupTestNamespace, Name: ex.Name}, got))
		})
	}
}

func TestStoreExecCleanupSkipsUnaffectedResources(t *testing.T) {
	ctx := context.Background()
	scheme := cleanupTestScheme(t)

	tests := []struct {
		name string
		ex   *shopv1.StoreExec
	}{
		{
			name: "cron done",
			ex: func() *shopv1.StoreExec {
				ex := storeExecForCleanup("cron-done", shopv1.ExecStateDone, time.Now().Add(-2*time.Hour))
				ex.Spec.CronSchedule = "*/5 * * * *"
				ex.Spec.CleanupPeriodSuccessfulExec = &metav1.Duration{Duration: time.Hour}
				return ex
			}(),
		},
		{
			name: "still running",
			ex: func() *shopv1.StoreExec {
				ex := storeExecForCleanup("running", shopv1.ExecStateRunning, time.Now().Add(-2*time.Hour))
				ex.Spec.CleanupPeriodSuccessfulExec = &metav1.Duration{Duration: time.Hour}
				ex.Spec.CleanupPeriodErrorExec = &metav1.Duration{Duration: time.Hour}
				return ex
			}(),
		},
		{
			name: "done with successful period disabled",
			ex: func() *shopv1.StoreExec {
				ex := storeExecForCleanup("done-no-period", shopv1.ExecStateDone, time.Now().Add(-2*time.Hour))
				ex.Spec.CleanupPeriodSuccessfulExec = &metav1.Duration{Duration: 0}
				ex.Spec.CleanupPeriodErrorExec = &metav1.Duration{Duration: time.Hour}
				return ex
			}(),
		},
		{
			name: "error with error period disabled",
			ex: func() *shopv1.StoreExec {
				ex := storeExecForCleanup("error-no-period", shopv1.ExecStateError, time.Now().Add(-2*time.Hour))
				ex.Spec.CleanupPeriodSuccessfulExec = &metav1.Duration{Duration: time.Hour}
				ex.Spec.CleanupPeriodErrorExec = &metav1.Duration{Duration: 0}
				return ex
			}(),
		},
		{
			name: "done without usable finish time",
			ex: func() *shopv1.StoreExec {
				ex := storeExecForCleanup("done-no-timestamp", shopv1.ExecStateDone, time.Now().Add(-2*time.Hour))
				ex.Spec.CleanupPeriodSuccessfulExec = &metav1.Duration{Duration: time.Hour}
				for i := range ex.Status.Conditions {
					ex.Status.Conditions[i].LastTransitionTime = metav1.Time{}
					ex.Status.Conditions[i].LastUpdateTime = metav1.Time{}
				}
				return ex
			}(),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cl := fake.NewClientBuilder().
				WithScheme(scheme).
				WithObjects(tt.ex).
				Build()

			reconciler := StoreExecReconciler{Client: cl}

			result, handled, err := reconciler.reconcileStoreExecCleanup(ctx, tt.ex)

			require.NoError(t, err)
			assert.False(t, handled)
			assert.Equal(t, time.Duration(0), result.RequeueAfter)

			got := &shopv1.StoreExec{}
			require.NoError(t, cl.Get(ctx, types.NamespacedName{Namespace: cleanupTestNamespace, Name: tt.ex.Name}, got))
		})
	}
}

func TestStoreDebugInstanceSuccessfulCleanupUsesDurationAndGracePeriod(t *testing.T) {
	ctx := context.Background()
	scheme := cleanupTestScheme(t)

	tests := []struct {
		name          string
		objectName    string
		creationTime  time.Time
		expectDeleted bool
	}{
		{
			name:          "after duration and grace",
			objectName:    "debug-done-old",
			creationTime:  time.Now().Add(-3 * time.Hour),
			expectDeleted: true,
		},
		{
			name:          "before duration and grace",
			objectName:    "debug-done-new",
			creationTime:  time.Now().Add(-90 * time.Minute),
			expectDeleted: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			debugInstance := storeDebugInstanceForCleanup(tt.objectName, tt.creationTime)
			cl := fake.NewClientBuilder().
				WithScheme(scheme).
				WithObjects(debugInstance).
				Build()

			reconciler := StoreDebugInstanceReconciler{
				Client:             cl,
				CleanupGracePeriod: time.Hour,
			}

			result, handled, err := reconciler.reconcileSuccessfulStoreDebugInstanceCleanup(ctx, debugInstance)

			require.NoError(t, err)
			assert.True(t, handled)

			got := &shopv1.StoreDebugInstance{}
			err = cl.Get(ctx, types.NamespacedName{Namespace: cleanupTestNamespace, Name: debugInstance.Name}, got)
			if tt.expectDeleted {
				assert.True(t, k8serrors.IsNotFound(err))
				assert.Zero(t, result)
				return
			}

			require.NoError(t, err)
			assert.Greater(t, result.RequeueAfter, time.Duration(0))
		})
	}
}

func TestSuccessfulCleanupDisabledWithZeroPeriods(t *testing.T) {
	ctx := context.Background()
	scheme := cleanupTestScheme(t)

	ex := storeExecForCleanup("disabled-exec", shopv1.ExecStateDone, time.Now().Add(-2*time.Hour))
	ex.Spec.CleanupPeriodSuccessfulExec = &metav1.Duration{Duration: 0}
	ex.Spec.CleanupPeriodErrorExec = &metav1.Duration{Duration: 0}
	debugInstance := storeDebugInstanceForCleanup("disabled-debug", time.Now().Add(-3*time.Hour))
	cl := fake.NewClientBuilder().
		WithScheme(scheme).
		WithObjects(ex, debugInstance).
		Build()

	execReconciler := StoreExecReconciler{Client: cl}
	execResult, execHandled, err := execReconciler.reconcileStoreExecCleanup(ctx, ex)
	require.NoError(t, err)
	assert.False(t, execHandled)
	assert.Equal(t, time.Duration(0), execResult.RequeueAfter)

	debugReconciler := StoreDebugInstanceReconciler{Client: cl}
	debugResult, debugHandled, err := debugReconciler.reconcileSuccessfulStoreDebugInstanceCleanup(ctx, debugInstance)
	require.NoError(t, err)
	assert.False(t, debugHandled)
	assert.Equal(t, time.Duration(0), debugResult.RequeueAfter)

	gotExec := &shopv1.StoreExec{}
	require.NoError(t, cl.Get(ctx, types.NamespacedName{Namespace: cleanupTestNamespace, Name: ex.Name}, gotExec))

	gotDebugInstance := &shopv1.StoreDebugInstance{}
	require.NoError(t, cl.Get(ctx, types.NamespacedName{Namespace: cleanupTestNamespace, Name: debugInstance.Name}, gotDebugInstance))
}

func cleanupTestScheme(t *testing.T) *runtime.Scheme {
	t.Helper()

	scheme := runtime.NewScheme()
	require.NoError(t, shopv1.AddToScheme(scheme))

	return scheme
}

func storeExecForCleanup(name string, state shopv1.StatefulState, finishedAt time.Time) *shopv1.StoreExec {
	return &shopv1.StoreExec{
		ObjectMeta: metav1.ObjectMeta{
			Name:              name,
			Namespace:         cleanupTestNamespace,
			CreationTimestamp: metav1.NewTime(finishedAt.Add(-time.Hour)),
		},
		Spec: shopv1.StoreExecSpec{
			StoreRef: "test",
			Script:   "echo test",
		},
		Status: shopv1.StoreExecStatus{
			State: state,
			Conditions: []shopv1.ExecCondition{
				{
					Type:               state,
					LastTransitionTime: metav1.NewTime(finishedAt),
					LastUpdateTime:     metav1.NewTime(finishedAt),
					Message:            "Command finished",
					Status:             "True",
				},
			},
		},
	}
}

func storeDebugInstanceForCleanup(name string, creationTime time.Time) *shopv1.StoreDebugInstance {
	return &shopv1.StoreDebugInstance{
		ObjectMeta: metav1.ObjectMeta{
			Name:              name,
			Namespace:         cleanupTestNamespace,
			CreationTimestamp: metav1.NewTime(creationTime),
		},
		Spec: shopv1.StoreDebugInstanceSpec{
			StoreRef: "test",
			Duration: "1h",
		},
		Status: shopv1.StoreDebugInstanceStatus{
			State: shopv1.StoreDebugInstanceStateDone,
		},
	}
}
