package controller

import (
	"context"
	"fmt"
	"time"

	v1 "github.com/shopware/shopware-operator/api/v1"
	"github.com/shopware/shopware-operator/internal/cronjob"
	"github.com/shopware/shopware-operator/internal/deployment"
	"github.com/shopware/shopware-operator/internal/event"
	"github.com/shopware/shopware-operator/internal/hpa"
	"github.com/shopware/shopware-operator/internal/httproute"
	"github.com/shopware/shopware-operator/internal/ingress"
	"github.com/shopware/shopware-operator/internal/job"
	"github.com/shopware/shopware-operator/internal/k8s"
	"github.com/shopware/shopware-operator/internal/logging"
	"github.com/shopware/shopware-operator/internal/pdb"
	"github.com/shopware/shopware-operator/internal/secret"
	"github.com/shopware/shopware-operator/internal/service"
	"go.uber.org/zap"
	appsv1 "k8s.io/api/apps/v1"
	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	networkingv1 "k8s.io/api/networking/v1"
	policy "k8s.io/api/policy/v1"
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
	"sigs.k8s.io/controller-runtime/pkg/predicate"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
	gatewayv1 "sigs.k8s.io/gateway-api/apis/v1"
)

var (
	noRequeue    = ctrl.Result{}
	shortRequeue = ctrl.Result{RequeueAfter: 5 * time.Second}
	longRequeue  = ctrl.Result{RequeueAfter: 2 * time.Minute}
)

// StoreReconciler reconciles a Store object
type StoreReconciler struct {
	client.Client
	Clientset            *kubernetes.Clientset
	RestConfig           *rest.Config
	Scheme               *runtime.Scheme
	Recorder             record.EventRecorder
	DisableServiceChecks bool
	EventHandlers        []event.EventHandler
	Logger               *zap.SugaredLogger
}

// SetupWithManager sets up the controller with the Manager.
func (r *StoreReconciler) SetupWithManager(mgr ctrl.Manager, logger *zap.SugaredLogger) error {
	skipStatusUpdates, err := NewSkipStatusUpdates(logger, &appsv1.Deployment{})
	if err != nil {
		return err
	}
	return ctrl.NewControllerManagedBy(mgr).
		For(&v1.Store{}).
		// We get triggerd by every update on the created resources, this leeads to high reconciles at the start.
		Owns(&corev1.Secret{}).
		Owns(&corev1.Service{}).
		Owns(&networkingv1.Ingress{}).
		Owns(&gatewayv1.HTTPRoute{}).
		Owns(&policy.PodDisruptionBudget{}).
		Owns(&appsv1.Deployment{}).
		Owns(&batchv1.Job{}).
		Owns(&batchv1.CronJob{}).
		// Skip status updates of all resources
		WithEventFilter(skipStatusUpdates).
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
		if store.Spec.Database.PasswordSecretRef.Name == secret.GetName() ||
			store.Spec.OpensearchSpec.PasswordSecretRef.Name == secret.GetName() ||
			store.Spec.ShopConfiguration.Fastly.TokenRef.Name == secret.GetName() {
			logging.FromContext(ctx).
				Infow(
					"Do reconcile on store because db/opensearch/fastly secret has changed",
					zap.String("store", store.Name),
					zap.String("secret", secret.GetName()),
					zap.String("secret-namespace", secret.GetNamespace()),
				)
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

//+kubebuilder:rbac:groups=shop.shopware.com,namespace=default,resources=stores,verbs=get;list;watch;create;update;patch
//+kubebuilder:rbac:groups=shop.shopware.com,namespace=default,resources=stores/status,verbs=get;update;patch
//+kubebuilder:rbac:groups=shop.shopware.com,namespace=default,resources=stores/finalizers,verbs=update
//+kubebuilder:rbac:groups="",namespace=default,resources=secrets,verbs=get;list;watch;create;patch
//+kubebuilder:rbac:groups="",namespace=default,resources=services,verbs=get;list;watch;create;patch
//+kubebuilder:rbac:groups="",namespace=default,resources=pods,verbs=get;list;watch;
//+kubebuilder:rbac:groups="apps",namespace=default,resources=deployments,verbs=get;list;watch;create;patch
//+kubebuilder:rbac:groups="batch",namespace=default,resources=jobs,verbs=get;list;watch;create;delete
//+kubebuilder:rbac:groups="networking.k8s.io",namespace=default,resources=ingresses,verbs=get;list;watch;create;patch
//+kubebuilder:rbac:groups="gateway.networking.k8s.io",namespace=default,resources=httproutes,verbs=get;list;watch;create;patch
//+kubebuilder:rbac:groups="policy",namespace=default,resources=poddisruptionbudgets,verbs=get;list;watch;create;patch
//+kubebuilder:rbac:groups="batch",namespace=default,resources=cronjobs,verbs=get;patch;list;watch;create;delete

func (r *StoreReconciler) Reconcile(
	ctx context.Context,
	req ctrl.Request,
) (rr ctrl.Result, err error) {
	log := r.Logger.
		With(zap.String("namespace", req.Namespace)).
		With(zap.String("name", req.Name))

	// Put logger in context for this reconcile
	ctx = logging.WithLogger(ctx, log)
	log.Info("Reconciling store")

	store, err := k8s.GetStore(ctx, r.Client, req.NamespacedName)
	if err != nil {
		if k8serrors.IsNotFound(err) {
			log.Info("Don't reconcile anymore, because resource was not found")
			return ctrl.Result{Requeue: false}, nil
		}
		log.Errorw("get CR", zap.Error(err))
		return rr, nil
	}

	// We don't need yet finalizers
	// if store.ObjectMeta.DeletionTimestamp != nil {
	// 	return rr, r.applyFinalizers(ctx, store)
	// }

	if !store.DeletionTimestamp.IsZero() {
		return shortRequeue, nil
	}

	if err := r.doReconcile(ctx, store); err != nil {
		log.Errorw("reconcile", zap.Error(err))
		return rr, nil
	}

	log.Debug("Reconcile finished, run status update")

	if err := r.reconcileCRStatus(ctx, store, err); err != nil {
		log.Errorw("failed to update status", zap.Error(err))
	}

	if store.IsState(v1.StateReady) {
		log.Info("Reconcile finished, schedule long Reconcile")
		return longRequeue, nil
	}

	log.Info("Schedule short Reconcile, because store is not ready yet")
	return shortRequeue, nil
}

func (r *StoreReconciler) doReconcile(
	ctx context.Context,
	store *v1.Store,
) error {
	log := logging.FromContext(ctx)
	log.Info("Do reconcile on store")

	log.Debug("reconcile ingress")
	if err := r.reconcileIngress(ctx, store); err != nil {
		return fmt.Errorf("ingress: %w", err)
	}

	log.Debug("reconcile gateway httproute")
	if err := r.reconcileHTTPRoute(ctx, store); err != nil {
		return fmt.Errorf("httproute: %w", err)
	}

	log.Debug("reconcile pdb")
	if err := r.reconcilePDB(ctx, store); err != nil {
		return fmt.Errorf("pdb: %w", err)
	}

	if store.IsState(v1.StateEmpty, v1.StateWait) {
		log.Info("skip some resources because s3/db/fastly/opensearch not ready or state is empty")
		return nil
	}

	log.Debug("reconcile app secrets")
	if err := r.ensureAppSecrets(ctx, store); err != nil {
		return fmt.Errorf("app secrets: %w", err)
	}

	// State Setup
	if store.IsState(v1.StateSetup, v1.StateSetupError) {
		if store.IsState(v1.StateSetupError) {
			log.Warn("Setup job has errors check setup logs. Waiting for new Image")
		}

		if err := r.reconcileSetupJob(ctx, store); err != nil {
			return fmt.Errorf("setup: %w", err)
		}
		log.Info("Wait for setup to finish")
		return nil
	}

	// State Initializing
	if store.IsState(v1.StateInitializing) {
		log.Info("reconcile deployment for initializing state")
		if err := r.reconcileDeployment(ctx, store); err != nil {
			return fmt.Errorf("deployment: %w", err)
		}
		return nil
	}

	// State Update
	if store.IsState(v1.StateMigration, v1.StateMigrationError) {
		if store.IsState(v1.StateMigrationError) {
			log.Warn("Migration job has errors check migration logs. Waiting for new Image")
		}
		if err := r.reconcileMigrationJob(ctx, store); err != nil {
			return fmt.Errorf("migration: %w", err)
		}

		store.Spec.ScheduledTask.Suspend = true
		log.Info("Overwrite Suspend for ScheduledTask because of migration")
		if err := r.reconcileScheduledTask(ctx, store); err != nil {
			return fmt.Errorf("cronjob: %w", err)
		}

		log.Info("wait for migration to finish")
		return nil
	}

	// Should be optional because we check the image in the status and switch to migration state. This should
	// also be the prefired way to update a store. But might cause issues needs more testing.
	if store.IsState(v1.StateReady) {
		if store.Status.CurrentImageTag != store.Spec.Container.Image {
			log.Info("wait for migration to finish")
			if err := r.reconcileMigrationJob(ctx, store); err != nil {
				return fmt.Errorf("migration: %w", err)
			}
			return nil
		}
	}

	// When setup job is still running we will kill it now. This happens when sidecars are used
	// EDIT: This makes more problems then it will help. So we process the way of terminating to
	// the user to close all sidecars correctly.
	// Check if sidecars are active
	// if len(store.Spec.Container.ExtraContainers) > 0 && !store.Spec.DisableJobDeletion {
	// 	log.Info("Delete setup/migration job if they are finished because sidecars are used")
	// 	if err := r.completeJobs(ctx, store); err != nil {
	// 		log.Errorw("Can't cleanup setup and migration jobs", zap.Error(err))
	// 	}
	// }

	log.Debug("reconcile deployment")
	if err := r.reconcileDeployment(ctx, store); err != nil {
		return fmt.Errorf("deployment: %w", err)
	}

	log.Debug("reconcile services")
	if err := r.reconcileServices(ctx, store); err != nil {
		return fmt.Errorf("service: %w", err)
	}

	log.Debug("reconcile CronJob scheduledTask")
	if err := r.reconcileScheduledTask(ctx, store); err != nil {
		return fmt.Errorf("cronjob: %w", err)
	}

	log.Debug("reconcile horizontalPodAutoscaler")
	if err := r.reconcileHoizontalPodAutoscaler(ctx, store); err != nil {
		return fmt.Errorf("hpa: %w", err)
	}

	return nil
}

// func (r *StoreReconciler) applyFinalizers(ctx context.Context, store *v1.Store) error {
// 	log := logging.FromContext(ctx).WithName(store.Name)
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
// 	// 			log.Errorw("failed to run finalizer", "finalizer", f)
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
// 			log.Errorw("Client.Update failed")
// 		}
// 		return err
// 	})
// }

func (r *StoreReconciler) ensureAppSecrets(ctx context.Context, store *v1.Store) (err error) {
	storeSecret, err := secret.EnsureStoreSecret(ctx, r.Client, r.Recorder, store)
	if err != nil {
		return fmt.Errorf("app secrets: %w", err)
	}

	if err := k8s.EnsureObjectWithHash(ctx, r.Client, nil, storeSecret, r.Scheme); err != nil {
		return fmt.Errorf("ensure store secret: %w", err)
	}

	return nil
}

func (r *StoreReconciler) reconcileServices(ctx context.Context, store *v1.Store) (err error) {
	objs := []*corev1.Service{
		service.StorefrontService(*store),
		service.AdminService(*store),
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
	if !store.Spec.Network.EnabledIngress {
		if err := ingress.DeleteStoreIngress(ctx, r.Client, *store); err != nil {
			return fmt.Errorf("delete ingress: %w", err)
		}
		return nil
	}

	var changed bool
	obj := ingress.StoreIngress(*store)

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

func (r *StoreReconciler) reconcileHTTPRoute(ctx context.Context, store *v1.Store) (err error) {
	if !store.Spec.Network.EnabledGateway {
		if err := httproute.DeleteStoreHTTPRoute(ctx, r.Client, *store); err != nil {
			return fmt.Errorf("delete httproute: %w", err)
		}
		return nil
	}

	var changed bool
	obj := httproute.StoreHTTPRoute(*store)

	if changed, err = k8s.HasObjectChanged(ctx, r.Client, obj); err != nil {
		return fmt.Errorf("reconcile unready httproute: %w", err)
	}

	if changed {
		r.Recorder.Event(store, "Normal", "Diff httproute hash",
			fmt.Sprintf("Update Store %s httproute in namespace %s. Diff hash",
				store.Name,
				store.Namespace))
		if err := k8s.EnsureHTTPRoute(ctx, r.Client, store, obj, r.Scheme, true); err != nil {
			return fmt.Errorf("reconcile unready httproute: %w", err)
		}
	}

	return nil
}

func (r *StoreReconciler) reconcilePDB(ctx context.Context, store *v1.Store) (err error) {
	var changed bool

	objs := []*policy.PodDisruptionBudget{
		pdb.AdminPDB(*store),
		pdb.StorefrontPDB(*store),
		pdb.WorkerPDB(*store),
	}

	for _, obj := range objs {
		if changed, err = k8s.HasObjectChanged(ctx, r.Client, obj); err != nil {
			return fmt.Errorf("reconcile unready pdb: %w", err)
		}

		if changed {
			r.Recorder.Event(store, "Normal", "Diff pdb hash",
				fmt.Sprintf("Update Store %s pdb in namespace %s. Diff hash",
					store.Name,
					store.Namespace))
			if err := k8s.EnsurePDB(ctx, r.Client, store, obj, r.Scheme, true); err != nil {
				return fmt.Errorf("reconcile unready pdb: %w", err)
			}
		}
	}

	return nil
}

func (r *StoreReconciler) reconcileDeployment(ctx context.Context, store *v1.Store) (err error) {
	var changed bool

	objs := []*appsv1.Deployment{
		deployment.StorefrontDeployment(*store),
		deployment.AdminDeployment(*store),
		deployment.WorkerDeployment(*store),
	}

	for _, obj := range objs {
		if changed, err = k8s.HasObjectChanged(ctx, r.Client, obj); err != nil {
			return fmt.Errorf("reconcile unready deployment: %w", err)
		}

		if changed {
			r.Recorder.Event(store, "Normal", "Diff deployment hash",
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
	obj := hpa.StoreHPA(*store)

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
	obj := job.MigrationJob(*store)

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
	obj := job.SetupJob(*store)

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

func (r *StoreReconciler) reconcileScheduledTask(ctx context.Context, store *v1.Store) (err error) {
	var changed bool
	obj := cronjob.ScheduledTaskJob(*store)

	if changed, err = k8s.HasObjectChanged(ctx, r.Client, obj); err != nil {
		return fmt.Errorf("reconcile unready setup job: %w", err)
	}

	if changed {
		r.Recorder.Event(store, "Normal", "Diff setup job hash",
			fmt.Sprintf("Update Store %s scheduled task job in namespace %s. Diff hash",
				store.Name,
				store.Namespace))
		if err := k8s.EnsureCronJob(ctx, r.Client, store, obj, r.Scheme, true); err != nil {
			return fmt.Errorf("reconcile unready scheduled task job: %w", err)
		}
	}

	return nil
}
