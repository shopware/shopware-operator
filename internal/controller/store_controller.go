package controller

import (
	"context"
	"fmt"
	"time"

	v1 "github.com/shopware/shopware-operator/api/v1"
	"github.com/shopware/shopware-operator/internal/deployment"
	"github.com/shopware/shopware-operator/internal/hpa"
	"github.com/shopware/shopware-operator/internal/ingress"
	"github.com/shopware/shopware-operator/internal/job"
	"github.com/shopware/shopware-operator/internal/k8s"
	"github.com/shopware/shopware-operator/internal/secret"
	"github.com/shopware/shopware-operator/internal/service"
	appsv1 "k8s.io/api/apps/v1"
	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/record"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/builder"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/predicate"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
)

// StoreReconciler reconciles a Store object
type StoreReconciler struct {
	client.Client
	Clientset            *kubernetes.Clientset
	RestConfig           *rest.Config
	Scheme               *runtime.Scheme
	Recorder             record.EventRecorder
	DisableServiceChecks bool
}

// SetupWithManager sets up the controller with the Manager.
func (r *StoreReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&v1.Store{}).
		Owns(&corev1.Secret{}).
		Owns(&appsv1.Deployment{}).
		Owns(&batchv1.Job{}).
		// This will watch the db secret and run a reconcile if the db secret will change.
		Watches(
			&corev1.Secret{},
			handler.EnqueueRequestsFromMapFunc(r.findStoreForReconcile),
			builder.WithPredicates(predicate.ResourceVersionChangedPredicate{}),
		).
		Complete(r)
}

func (r *StoreReconciler) findStoreForReconcile(
	ctx context.Context,
	secret client.Object,
) []reconcile.Request {
	stores := &v1.StoreList{}
	err := r.List(ctx, stores)
	if err != nil {
		return []reconcile.Request{}
	}

	var requests []reconcile.Request
	for _, store := range stores.Items {
		if store.Spec.Database.PasswordSecretRef.Name == secret.GetName() {
			log.FromContext(ctx).
				Info("Do reconcile on store because db secret has changed", "Store", store.Name)
			requests = append(requests, reconcile.Request{
				NamespacedName: types.NamespacedName{
					Namespace: store.Namespace,
					Name:      store.Name,
				},
			})
		}
	}

	return requests
}

//+kubebuilder:rbac:groups=shop.shopware.com,namespace=default,resources=stores,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=shop.shopware.com,namespace=default,resources=stores/status,verbs=get;update;patch
//+kubebuilder:rbac:groups=shop.shopware.com,namespace=default,resources=stores/finalizers,verbs=update
//+kubebuilder:rbac:groups="",namespace=default,resources=secrets,verbs=get;list;watch;create;patch
//+kubebuilder:rbac:groups="",namespace=default,resources=services,verbs=get;list;watch;create;patch
//+kubebuilder:rbac:groups="",namespace=default,resources=pods,verbs=get;list;watch;
//+kubebuilder:rbac:groups="apps",namespace=default,resources=deployments,verbs=get;list;watch;create;patch
//+kubebuilder:rbac:groups="batch",namespace=default,resources=jobs,verbs=get;list;watch;create;delete
//+kubebuilder:rbac:groups="networking.k8s.io",namespace=default,resources=ingresses,verbs=get;list;watch;create;patch

func (r *StoreReconciler) Reconcile(
	ctx context.Context,
	req ctrl.Request,
) (rr ctrl.Result, err error) {
	log := log.FromContext(ctx)

	rr = ctrl.Result{RequeueAfter: 10 * time.Second}

	var store *v1.Store
	defer func() {
		if err := r.reconcileCRStatus(ctx, store, err); err != nil {
			log.Error(err, "failed to update status")
		}
	}()

	store, err = k8s.GetStore(ctx, r.Client, req.NamespacedName)
	if err != nil {
		if k8serrors.IsNotFound(err) {
			return ctrl.Result{}, nil
		}
		log.Error(err, "get CR")
		return rr, nil
	}

	// We don't need yet finalizers
	// if store.ObjectMeta.DeletionTimestamp != nil {
	// 	return rr, r.applyFinalizers(ctx, store)
	// }

	if err := r.doReconcile(ctx, store); err != nil {
		log.Error(err, "reconcile")
		return rr, nil
	}

	log.Info("Reconcile finished")
	return rr, nil
}

func (r *StoreReconciler) doReconcile(
	ctx context.Context,
	store *v1.Store,
) error {
	log := log.FromContext(ctx).
		WithName(store.Name).
		WithValues("state", store.Status.State)
	log.Info("Do reconcile on store")

	log.Info("reconcile app secrets")
	if err := r.ensureAppSecrets(ctx, store); err != nil {
		return fmt.Errorf("app secrets: %w", err)
	}

	if store.IsState(v1.StateEmpty, v1.StateWait) {
		log.Info("skip reconcile because s3/db not ready or state is empty")
		return nil
	}

	// State Setup
	if store.IsState(v1.StateSetup) {
		if err := r.reconcileSetupJob(ctx, store); err != nil {
			return fmt.Errorf("setup: %w", err)
		}
		log.Info("Wait for setup to finish")
		return nil
	}

	// State Initializing
	if store.IsState(v1.StateInitializing) {
		// When setup job is still running we will kill it now. This happens when sidecars are used
		// EDIT: This makes more problems then it will help. So we process the way of terminating to
		// the user to close all sidecars correctly.
		// Check if sidecars are active
		if len(store.Spec.Container.ExtraContainers) > 0 {
			log.Info("Delete setup/migration job if they are finished because sidecars are used")
			if err := r.completeJobs(ctx, store); err != nil {
				log.Error(err, "Can't cleanup setup and migration jobs")
			}
		}

		log.Info("reconcile deployment")
		if err := r.reconcileDeployment(ctx, store); err != nil {
			return fmt.Errorf("deployment: %w", err)
		}
		log.Info("Wait for deployment to finish")
		return nil
	}

	// State Update
	if store.IsState(v1.StateMigration) {
		if err := r.reconcileMigrationJob(ctx, store); err != nil {
			return fmt.Errorf("migration: %w", err)
		}
		log.Info("wait for migration to finish")
		return nil
	}

	if store.IsState(v1.StateReady) {
		if store.Status.CurrentImageTag != store.Spec.Container.Image {
			log.Info("wait for migration to finish")
			if err := r.reconcileMigrationJob(ctx, store); err != nil {
				return fmt.Errorf("migration: %w", err)
			}
			log.Info("wait for migration to finish")
			return nil
		}
	}

	log.Info("reconcile deployment")
	if err := r.reconcileDeployment(ctx, store); err != nil {
		return fmt.Errorf("deployment: %w", err)
	}

	log.Info("reconcile services")
	if err := r.reconcileServices(ctx, store); err != nil {
		return fmt.Errorf("service: %w", err)
	}

	log.Info("reconcile ingress")
	if err := r.reconcileIngress(ctx, store); err != nil {
		return fmt.Errorf("service: %w", err)
	}

	log.Info("reconcile horizontalPodAutoscaler")
	if err := r.reconcileHoizontalPodAutoscaler(ctx, store); err != nil {
		return fmt.Errorf("hpa: %w", err)
	}

	return nil
}

// func (r *StoreReconciler) applyFinalizers(ctx context.Context, store *v1.Store) error {
// 	log := log.FromContext(ctx).WithName(store.Name)
// 	log.Info("Applying finalizers")
//
// 	var err error
//
// 	// finalizers := []string{}
// 	// for _, f := range store.GetFinalizers() {
// 	// 	switch f {
// 	// 	case "delete-mysql-pods-in-order":
// 	// 		err = r.deleteMySQLPods(ctx, store)
// 	// 	case "delete-ssl":
// 	// 		err = r.deleteCerts(ctx, store)
// 	// 	}
// 	//
// 	// 	if err != nil {
// 	// 		switch err {
// 	// 		case psrestore.ErrWaitingTermination:
// 	// 			log.Info("waiting for pods to be deleted", "finalizer", f)
// 	// 		default:
// 	// 			log.Error(err, "failed to run finalizer", "finalizer", f)
// 	// 		}
// 	// 		finalizers = append(finalizers, f)
// 	// 	}
// 	// }
//
// 	//store.SetFinalizers(finalizers)
//
// 	return k8sretry.RetryOnConflict(k8sretry.DefaultRetry, func() error {
// 		err = r.Client.Update(ctx, store)
// 		if err != nil {
// 			log.Error(err, "Client.Update failed")
// 		}
// 		return err
// 	})
// }

func (r *StoreReconciler) ensureAppSecrets(ctx context.Context, store *v1.Store) (err error) {
	nn := types.NamespacedName{
		Namespace: store.Namespace,
		Name:      store.GetSecretName(),
	}

	storeSecret := new(corev1.Secret)

	dbSecret := new(corev1.Secret)
	if err := r.Client.Get(ctx, types.NamespacedName{
		Namespace: store.Namespace,
		Name:      store.Spec.Database.PasswordSecretRef.Name,
	}, dbSecret); err != nil {
		if k8serrors.IsNotFound(err) {
			r.Recorder.Event(store, "Warning", "DB secret not found",
				fmt.Sprintf("Missing database secret for Store %s in namespace %s",
					store.Name,
					store.Namespace))
			return nil
		}
		return fmt.Errorf("can't read database secret: %w", err)
	}

	if err = r.Get(ctx, nn, storeSecret); client.IgnoreNotFound(err) != nil {
		return fmt.Errorf("get store secret: %w", err)
	}
	if err = secret.GenerateStoreSecret(ctx, store, storeSecret, dbSecret.Data[store.Spec.Database.PasswordSecretRef.Key]); err != nil {
		return fmt.Errorf("fill store secret: %w", err)
	}
	storeSecret.Name = store.GetSecretName()
	storeSecret.Namespace = store.Namespace

	if err := k8s.EnsureObjectWithHash(ctx, r.Client, nil, storeSecret, r.Scheme); err != nil {
		return fmt.Errorf("ensure store secret: %w", err)
	}

	return nil
}

func (r *StoreReconciler) reconcileServices(ctx context.Context, store *v1.Store) (err error) {
	objs := []*corev1.Service{
		service.StorefrontService(store),
		service.AdminService(store),
	}

	var changed bool
	for _, obj := range objs {
		if changed, err = k8s.HasObjectChanged(ctx, r.Client, obj); err != nil {
			return fmt.Errorf("reconcile unready svc: %w", err)
		}

		if changed {
			r.Recorder.Event(store, "Normal", "Diff service hash",
				fmt.Sprintf("Update Store %s service in namespace %s for %s. Diff hash",
					store.Name,
					store.Namespace,
					obj.Labels["app"]))
			if err := k8s.EnsureService(ctx, r.Client, store, obj, r.Scheme, true); err != nil {
				return fmt.Errorf("reconcile unready services: %w", err)
			}
		}
	}

	return nil
}

func (r *StoreReconciler) reconcileIngress(ctx context.Context, store *v1.Store) (err error) {
	var changed bool
	obj := ingress.StoreIngress(store)

	if changed, err = k8s.HasObjectChanged(ctx, r.Client, obj); err != nil {
		return fmt.Errorf("reconcile unready ingress: %w", err)
	}

	if changed {
		r.Recorder.Event(store, "Normal", "Diff ingress hash",
			fmt.Sprintf("Update Store %s ingress in namespace %s. Diff hash",
				store.Name,
				store.Namespace))
		if err := k8s.EnsureIngress(ctx, r.Client, store, obj, r.Scheme, true); err != nil {
			return fmt.Errorf("reconcile unready ingress: %w", err)
		}
	}

	return nil
}

func (r *StoreReconciler) reconcileDeployment(ctx context.Context, store *v1.Store) (err error) {
	var changed bool

	objs := []*appsv1.Deployment{
		deployment.StorefrontDeployment(store),
		deployment.AdminDeployment(store),
		deployment.WorkerDeployment(store),
	}

	for _, obj := range objs {

		if changed, err = k8s.HasObjectChanged(ctx, r.Client, obj); err != nil {
			return fmt.Errorf("reconcile unready deployment: %w", err)
		}

		if changed {
			r.Recorder.Event(store, "Normal", "Diff ingress hash",
				fmt.Sprintf("Update Store %s deployment in namespace %s for %s. Diff hash",
					store.Name,
					store.Namespace,
					obj.Labels["app"]))
			if err := k8s.EnsureDeployment(ctx, r.Client, store, obj, r.Scheme, true); err != nil {
				return fmt.Errorf("reconcile unready deployment: %w", err)
			}
		}
	}

	return nil
}

func (r *StoreReconciler) reconcileHoizontalPodAutoscaler(
	ctx context.Context,
	store *v1.Store,
) (err error) {
	if !store.Spec.HorizontalPodAutoscaler.Enabled {
		return nil
	}

	var changed bool
	obj := hpa.StoreHPA(store)

	if changed, err = k8s.HasObjectChanged(ctx, r.Client, obj); err != nil {
		return fmt.Errorf("reconcile unready deployment: %w", err)
	}

	if changed {
		r.Recorder.Event(store, "Normal", "Diff hpa hash",
			fmt.Sprintf("Update Store %s hpa in namespace %s. Diff hash",
				store.Name,
				store.Namespace))
		if err := k8s.EnsureHPA(ctx, r.Client, store, obj, r.Scheme, true); err != nil {
			return fmt.Errorf("reconcile unready hpa: %w", err)
		}
	}

	return nil
}

func (r *StoreReconciler) reconcileMigrationJob(ctx context.Context, store *v1.Store) (err error) {
	var changed bool
	obj := job.MigrationJob(store)

	if changed, err = k8s.HasObjectChanged(ctx, r.Client, obj); err != nil {
		return fmt.Errorf("reconcile unready migrate job: %w", err)
	}

	if changed {
		r.Recorder.Event(store, "Normal", "Diff migrate job hash",
			fmt.Sprintf("Update Store %s migrate job in namespace %s. Diff hash",
				store.Name,
				store.Namespace))
		if err := k8s.EnsureJob(ctx, r.Client, store, obj, r.Scheme, true); err != nil {
			return fmt.Errorf("reconcile unready migrate job: %w", err)
		}
	}

	return nil
}

func (r *StoreReconciler) reconcileSetupJob(ctx context.Context, store *v1.Store) (err error) {
	var changed bool
	obj := job.SetupJob(store)

	if changed, err = k8s.HasObjectChanged(ctx, r.Client, obj); err != nil {
		return fmt.Errorf("reconcile unready setup job: %w", err)
	}

	if changed {
		r.Recorder.Event(store, "Normal", "Diff setup job hash",
			fmt.Sprintf("Update Store %s setup job in namespace %s. Diff hash",
				store.Name,
				store.Namespace))
		if err := k8s.EnsureJob(ctx, r.Client, store, obj, r.Scheme, true); err != nil {
			return fmt.Errorf("reconcile unready setup job: %w", err)
		}
	}

	return nil
}

func (r *StoreReconciler) completeJobs(ctx context.Context, store *v1.Store) error {
	done, err := job.IsSetupJobCompleted(ctx, r.Client, store)
	if err != nil {
		return err
	}
	// The job is not completed because active containers are running
	if !done {
		if err = job.DeleteSetupJob(ctx, r.Client, store); err != nil {
			return err
		}
	}
	done, err = job.IsMigrationJobCompleted(ctx, r.Client, store)
	if err != nil {
		return err
	}
	// The job is not completed because active containers are running
	if !done {
		if err = job.DeleteAllMigrationJobs(ctx, r.Client, store); err != nil {
			return err
		}
	}
	return nil
}
