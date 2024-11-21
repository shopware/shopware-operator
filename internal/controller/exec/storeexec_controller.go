package exec

import (
	"context"

	"time"

	v1 "github.com/shopware/shopware-operator/api/v1"
	"github.com/shopware/shopware-operator/internal/k8s"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime"
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

// +kubebuilder:rbac:groups=shop.shopware.com,resources=storeexecs,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=shop.shopware.com,resources=storeexecs/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=shop.shopware.com,resources=storeexecs/finalizers,verbs=update
func (r *StoreExecReconciler) Reconcile(ctx context.Context, req ctrl.Request) (rr ctrl.Result, err error) {
	log := log.FromContext(ctx)

	rr = ctrl.Result{RequeueAfter: 10 * time.Second}

	var ex *v1.StoreExec
	defer func() {
		if err := r.reconcileCRStatus(ctx, ex, err); err != nil {
			log.Error(err, "failed to update status")
		}
	}()

	ex, err = k8s.GetStoreExec(ctx, r.Client, req.NamespacedName)
	if err != nil {
		if k8serrors.IsNotFound(err) {
			return ctrl.Result{}, nil
		}
		log.Error(err, "get CR")
		return rr, nil
	}

	if err := r.doReconcile(ctx, ex); err != nil {
		log.Error(err, "reconcile")
		return rr, nil
	}

	log.Info("Reconcile finished")
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
	ex *v1.StoreExec,
) error {
	log := log.FromContext(ctx).
		WithName(ex.Name).
		WithValues("state", ex.Status.State)
	log.Info("Do reconcile on store-exec")

	if ex.IsState(v1.ExecStateEmpty) {
		log.Info("skip reconcile because state is empty")
		return nil
	}

	return nil
}
