package controller

import (
	"context"
	"fmt"
	"time"

	kedav1alpha1 "github.com/kedacore/keda/v2/apis/keda/v1alpha1"
	v1 "github.com/shopware/shopware-operator/api/v1"
	"github.com/shopware/shopware-operator/internal/event"
	"github.com/shopware/shopware-operator/internal/k8s"
	"github.com/shopware/shopware-operator/internal/logging"
	"github.com/shopware/shopware-operator/internal/manager"
	"github.com/shopware/shopware-operator/internal/manager/base"
	"github.com/shopware/shopware-operator/internal/metrics"
	"go.uber.org/zap"
	appsv1 "k8s.io/api/apps/v1"
	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	networkingv1 "k8s.io/api/networking/v1"
	policy "k8s.io/api/policy/v1"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
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
	EnableKeda           bool
	OperatorMetricsURL   string
	EventHandlers        []event.EventHandler
	Logger               *zap.SugaredLogger
	StateManager         *manager.StoreStateManager
}

func (r *StoreReconciler) stateManager() *manager.StoreStateManager {
	if r.StateManager == nil {
		r.StateManager = manager.NewStoreStateManager(&base.Base{
			Client:               r.Client,
			Clientset:            r.Clientset,
			RestConfig:           r.RestConfig,
			Scheme:               r.Scheme,
			Recorder:             r.Recorder,
			EventHandlers:        r.EventHandlers,
			DisableServiceChecks: r.DisableServiceChecks,
			EnableKeda:           r.EnableKeda,
			OperatorMetricsURL:   r.OperatorMetricsURL,
		})
	}
	return r.StateManager
}

// SetupWithManager sets up the controller with the Manager.
func (r *StoreReconciler) SetupWithManager(mgr ctrl.Manager, logger *zap.SugaredLogger) error {
	skipStatusUpdates, err := NewSkipStatusUpdates(logger, &appsv1.Deployment{})
	if err != nil {
		return err
	}
	controllerBuilder := ctrl.NewControllerManagedBy(mgr).
		For(&v1.Store{}).
		// We get triggered by every update on the created resources, this leads to high reconciles at the start.
		Owns(&corev1.Secret{}).
		Owns(&corev1.Service{}).
		Owns(&networkingv1.Ingress{})

	_, err = mgr.GetRESTMapper().RESTMapping(
		gatewayv1.SchemeGroupVersion.WithKind("HTTPRoute").GroupKind(),
		gatewayv1.SchemeGroupVersion.Version,
	)
	if err == nil {
		controllerBuilder = controllerBuilder.Owns(&gatewayv1.HTTPRoute{})
	} else if !meta.IsNoMatchError(err) {
		return fmt.Errorf("resolve HTTPRoute REST mapping: %w", err)
	}

	if r.EnableKeda {
		_, err = mgr.GetRESTMapper().RESTMapping(
			kedav1alpha1.SchemeGroupVersion.WithKind("ScaledObject").GroupKind(),
			kedav1alpha1.SchemeGroupVersion.Version,
		)
		if err == nil {
			controllerBuilder = controllerBuilder.Owns(&kedav1alpha1.ScaledObject{})
		} else if !meta.IsNoMatchError(err) {
			return fmt.Errorf("resolve ScaledObject REST mapping: %w", err)
		}
	}

	return controllerBuilder.
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
		if store.Namespace != secret.GetNamespace() {
			continue
		}
		if store.Spec.Database.PasswordSecretRef.Name == secret.GetName() ||
			store.Spec.Database.TLS.SecretName == secret.GetName() ||
			store.Spec.OpensearchSpec.PasswordSecretRef.Name == secret.GetName() ||
			store.Spec.ShopConfiguration.Fastly.TokenRef.Name == secret.GetName() ||
			store.Spec.AdminCredentials.UsernameSecretRef.Name == secret.GetName() ||
			store.Spec.AdminCredentials.PasswordSecretRef.Name == secret.GetName() {
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
//+kubebuilder:rbac:groups="",namespace=default,resources=pods/exec,verbs=create
//+kubebuilder:rbac:groups="apps",namespace=default,resources=deployments,verbs=get;list;watch;create;patch;delete
//+kubebuilder:rbac:groups="batch",namespace=default,resources=jobs,verbs=get;list;watch;create;delete
//+kubebuilder:rbac:groups="networking.k8s.io",namespace=default,resources=ingresses,verbs=get;list;watch;create;patch;delete
//+kubebuilder:rbac:groups="gateway.networking.k8s.io",namespace=default,resources=httproutes,verbs=get;list;watch;create;patch;delete
//+kubebuilder:rbac:groups="policy",namespace=default,resources=poddisruptionbudgets,verbs=get;list;watch;create;patch
//+kubebuilder:rbac:groups="batch",namespace=default,resources=cronjobs,verbs=get;patch;list;watch;create;delete
//+kubebuilder:rbac:groups="keda.sh",namespace=default,resources=scaledobjects,verbs=get;list;watch;create;patch;delete

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
			return ctrl.Result{}, nil
		}
		log.Errorw("get CR", zap.Error(err))
		return rr, nil
	}

	if !store.DeletionTimestamp.IsZero() {
		metrics.RemoveStoreMetrics(store)
		return shortRequeue, nil
	}

	if err := r.stateManager().ReconcileResources(ctx, store); err != nil {
		log.Errorw("reconcile", zap.Error(err))
		return rr, nil
	}

	log.Debug("Reconcile finished, run status update")

	if err := r.stateManager().ReconcileStatus(ctx, store, err); err != nil {
		log.Errorw("failed to update status", zap.Error(err))
	}

	if store.IsState(v1.StateReady) {
		log.Info("Reconcile finished, schedule long Reconcile")
		return longRequeue, nil
	}

	log.Info("Schedule short Reconcile, because store is not ready yet")
	return shortRequeue, nil
}
