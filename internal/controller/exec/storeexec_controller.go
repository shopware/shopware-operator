package exec

import (
	"context"
	"fmt"

	"time"

	v1 "github.com/shopware/shopware-operator/api/v1"
	"github.com/shopware/shopware-operator/internal/job"
	"github.com/shopware/shopware-operator/internal/k8s"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/tools/record"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"
)

type StoreExecReconciler struct {
	client.Client
	Scheme   *runtime.Scheme
	Recorder record.EventRecorder
}

// +kubebuilder:rbac:groups=shop.shopware.com,namespace=default,resources=storeexecs,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=shop.shopware.com,namespace=default,resources=storeexecs/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=shop.shopware.com,namespace=default,resources=storeexecs/finalizers,verbs=update
// +kubebuilder:rbac:groups=shop.shopware.com,namespace=default,resources=stores,verbs=get
// +kubebuilder:rbac:groups="",namespace=default,resources=secrets,verbs=get;list;watch;create;patch
// +kubebuilder:rbac:groups="",namespace=default,resources=pods,verbs=get;list;watch;
// +kubebuilder:rbac:groups="batch",namespace=default,resources=jobs,verbs=get;list;watch;create;delete
func (r *StoreExecReconciler) Reconcile(ctx context.Context, req ctrl.Request) (rr ctrl.Result, err error) {
	log := log.FromContext(ctx)

	rr = ctrl.Result{RequeueAfter: 1 * time.Second}

	var ex *v1.StoreExec
	var store *v1.Store
	defer func() {
		if err := r.reconcileCRStatus(ctx, store, ex, err); err != nil {
			log.Error(err, "failed to update status")
		}
	}()

	ex, err = k8s.GetStoreExec(ctx, r.Client, req.NamespacedName)
	if err != nil {
		if k8serrors.IsNotFound(err) {
			return rr, nil
		}
		log.Error(err, "get CR exec")
		return rr, nil
	}

	store, err = k8s.GetStore(ctx, r.Client, types.NamespacedName{
		Namespace: req.Namespace,
		Name:      ex.Spec.StoreRef,
	})
	if err != nil {
		if k8serrors.IsNotFound(err) {
			return rr, fmt.Errorf("store not found: %w", err)
		}
		log.Error(err, "get CR store")
		return rr, nil
	}

	if err := r.doReconcile(ctx, store, ex); err != nil {
		log.Error(err, "reconcile")
		return rr, nil
	}

	log.Info("Reconcile finished")
	rr.RequeueAfter = 1 * time.Second
	return rr, nil
}

// SetupWithManager sets up the controller with the Manager.
func (r *StoreExecReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&v1.StoreExec{}).
		Complete(r)
}

func (r *StoreExecReconciler) doReconcile(
	ctx context.Context,
	store *v1.Store,
	ex *v1.StoreExec,
) error {
	log := log.FromContext(ctx).
		WithName(ex.Name).
		WithValues("store", ex.Spec.StoreRef).
		WithValues("state", ex.Status.State)
	log.Info("Do reconcile on store-exec")

	if ex.IsState(v1.ExecStateEmpty) {
		log.Info("skip reconcile because state is empty")
		return nil
	}

	if err := r.reconcileJob(ctx, store, ex); err != nil {
		return fmt.Errorf("exec: %w", err)
	}

	log.Info("wait for exec to finish")

	return nil
}

func (r *StoreExecReconciler) reconcileJob(ctx context.Context, store *v1.Store, exec *v1.StoreExec) (err error) {
	var changed bool
	obj := job.CommandJob(store, exec)

	if changed, err = k8s.HasObjectChanged(ctx, r.Client, obj); err != nil {
		return fmt.Errorf("reconcile unready setup job: %w", err)
	}

	if changed {
		r.Recorder.Event(store, "Normal", "Diff command job hash",
			fmt.Sprintf("Update command Job %s in namespace %s. Diff hash",
				exec.Name,
				exec.Namespace))
		if err := k8s.EnsureJob(ctx, r.Client, exec, obj, r.Scheme, true); err != nil {
			return fmt.Errorf("reconcile unready setup job: %w", err)
		}
	}

	return nil
}