package controller

import (
	"context"
	"fmt"
	"slices"
	"time"

	kedav1alpha1 "github.com/kedacore/keda/v2/apis/keda/v1alpha1"
	v1 "github.com/shopware/shopware-operator/api/v1"
	"github.com/shopware/shopware-operator/internal/event"
	"github.com/shopware/shopware-operator/internal/k8s"
	"github.com/shopware/shopware-operator/internal/logging"
	"github.com/shopware/shopware-operator/internal/manager"
	"github.com/shopware/shopware-operator/internal/manager/base"
	"github.com/shopware/shopware-operator/internal/metrics"
	"github.com/shopware/shopware-operator/internal/tracing"
	"github.com/shopware/shopware-operator/internal/trigger"
	"github.com/shopware/shopware-operator/internal/util"
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
	noRequeue        = ctrl.Result{}
	shortRequeue     = ctrl.Result{RequeueAfter: 5 * time.Second}
	apiFailedRequeue = ctrl.Result{RequeueAfter: 20 * time.Second}
	longRequeue      = ctrl.Result{RequeueAfter: 15 * time.Minute}
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
	Triggers             *trigger.Recorder
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
	if r.Triggers == nil {
		r.Triggers = trigger.NewRecorder()
	}
	ownedBy := trigger.ControllerOwner("Store")
	owned := func(source string) builder.Predicates {
		return builder.WithPredicates(
			r.Triggers.Predicate(source, ownedBy, k8s.HasObjectChangedPredicate()),
		)
	}

	controllerBuilder := ctrl.NewControllerManagedBy(mgr).
		For(&v1.Store{}, builder.WithPredicates(
			r.Triggers.Predicate("Store", trigger.Self, predicate.Or(
				predicate.GenerationChangedPredicate{},
				predicate.LabelChangedPredicate{},
				predicate.AnnotationChangedPredicate{},
			)),
		)).
		Owns(&corev1.Secret{}, owned("Secret")).
		Owns(&corev1.Service{}, owned("Service")).
		Owns(&networkingv1.Ingress{}, owned("Ingress"))

	_, err := mgr.GetRESTMapper().RESTMapping(
		gatewayv1.SchemeGroupVersion.WithKind("HTTPRoute").GroupKind(),
		gatewayv1.SchemeGroupVersion.Version,
	)
	if err == nil {
		controllerBuilder = controllerBuilder.Owns(&gatewayv1.HTTPRoute{}, owned("HTTPRoute"))
	} else if !meta.IsNoMatchError(err) {
		return fmt.Errorf("resolve HTTPRoute REST mapping: %w", err)
	}

	if r.EnableKeda {
		_, err = mgr.GetRESTMapper().RESTMapping(
			kedav1alpha1.SchemeGroupVersion.WithKind("ScaledObject").GroupKind(),
			kedav1alpha1.SchemeGroupVersion.Version,
		)
		if err == nil {
			controllerBuilder = controllerBuilder.Owns(&kedav1alpha1.ScaledObject{}, owned("ScaledObject"))
		} else if !meta.IsNoMatchError(err) {
			return fmt.Errorf("resolve ScaledObject REST mapping: %w", err)
		}
	}

	return controllerBuilder.
		Owns(&policy.PodDisruptionBudget{}, owned("PodDisruptionBudget")).
		Owns(&appsv1.Deployment{}, owned("Deployment")).
		Owns(&batchv1.Job{}, owned("Job")).
		Owns(&batchv1.CronJob{}, owned("CronJob")).
		// This will watch the db secret and run a reconcile if the db secret will change.
		Watches(
			&corev1.Secret{},
			handler.EnqueueRequestsFromMapFunc(r.findStoreWithSecretChangeForReconcile),
			builder.WithPredicates(predicate.Or(
				predicate.ResourceVersionChangedPredicate{},
			)),
		).
		Complete(r)
}

func (r *StoreReconciler) findStoreWithSecretChangeForReconcile(
	ctx context.Context,
	secret client.Object,
) []reconcile.Request {
	stores := &v1.StoreList{}
	err := r.List(ctx, stores)
	if err != nil {
		return []reconcile.Request{}
	}

	r.Logger.Debugw("got secret update event", zap.String("secret", secret.GetName()))

	var requests []reconcile.Request
	for _, store := range stores.Items {
		if store.Namespace != secret.GetNamespace() {
			continue
		}

		// Own secret has changed, so we schedule a reconcile
		// Triggered by delete for example, or changes, or creates
		if store.GetSecretName() == secret.GetName() {
			s := types.NamespacedName{
				Namespace: store.Namespace,
				Name:      store.Name,
			}
			r.Triggers.Add(s, trigger.Trigger{
				Source: "StoreSecret",
				Event:  trigger.EventMapped,
				Name:   secret.GetName(),
				Reason: "store secret has changed",
			})
			return []reconcile.Request{{
				NamespacedName: s,
			}}
		}

		if store.Spec.Database.TLS.SecretName == secret.GetName() ||
			slices.Contains(util.GetReferencedSecretNames(store), secret.GetName()) {
			r.Logger.
				Debugw(
					"Do reconcile on store because referenced secret has changed",
					zap.String("store", store.Name),
					zap.String("secret", secret.GetName()),
					zap.String("secret-namespace", secret.GetNamespace()),
				)
			store := types.NamespacedName{
				Namespace: store.Namespace,
				Name:      store.Name,
			}
			r.Triggers.Add(store, trigger.Trigger{
				Source: "ReferencedSecret",
				Event:  trigger.EventMapped,
				Name:   secret.GetName(),
				Reason: "secretRef has changed",
			})
			requests = append(requests, reconcile.Request{NamespacedName: store})
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
//+kubebuilder:rbac:groups="policy",namespace=default,resources=poddisruptionbudgets,verbs=get;list;watch;create;patch;delete
//+kubebuilder:rbac:groups="batch",namespace=default,resources=cronjobs,verbs=get;patch;list;watch;create;delete
//+kubebuilder:rbac:groups="keda.sh",namespace=default,resources=scaledobjects,verbs=get;list;watch;create;patch;delete

func (r *StoreReconciler) Reconcile(
	ctx context.Context,
	req ctrl.Request,
) (rr ctrl.Result, err error) {
	ctx, span := tracing.StartReconcile(ctx, "Store", req)
	defer func() { tracing.EndReconcile(span, rr, &err) }()

	var requeueReason string
	defer func() {
		span.SetAttributes(tracing.AttrRequeueReason.String(requeueReason))
		r.Triggers.Finish(req.NamespacedName, requeueReason, rr, err)
	}()

	triggers := r.Triggers.Drain(req.NamespacedName)
	trigger.RecordOnSpan(span, triggers)

	log := r.Logger.
		With(zap.String("namespace", req.Namespace)).
		With(zap.String("name", req.Name))

	// Put logger in context for this reconcile
	ctx = logging.WithLogger(ctx, log)
	log = logging.FromContext(ctx)
	log.Infow("Reconciling store",
		zap.String("cause", trigger.Cause(triggers)),
		zap.Strings("triggers", trigger.Summaries(triggers)))

	store, err := k8s.GetStore(ctx, r.Client, req.NamespacedName)
	if err != nil {
		if k8serrors.IsNotFound(err) {
			log.Info("Don't reconcile anymore, because resource was not found")
			r.Triggers.Forget(req.NamespacedName)
			requeueReason = "store not found, no requeue"
			return ctrl.Result{}, nil
		}
		log.Errorw("get CR", zap.Error(err))
		tracing.RecordError(span, err)
		requeueReason = "getting store failed, short requeue"
		return apiFailedRequeue, nil
	}
	span.SetAttributes(tracing.AttrState.String(string(store.Status.State)))

	if !store.DeletionTimestamp.IsZero() {
		metrics.RemoveStoreMetrics(store)
		requeueReason = "store is being deleted"
		return shortRequeue, nil
	}

	if err := r.stateManager().ReconcileResources(ctx, store); err != nil {
		log.Errorw("reconcile", zap.Error(err))
		tracing.RecordError(span, err)
		requeueReason = "reconciling resources failed, short requeue"
		return shortRequeue, nil
	}

	log.Debug("Reconcile finished, run status update")

	if err := r.stateManager().ReconcileStatus(ctx, store, err); err != nil {
		log.Errorw("failed to update status", zap.Error(err))
		tracing.RecordError(span, err)
	}

	if !store.IsState(v1.StateReady) {
		log.Info("Schedule short Reconcile, because store is not ready yet")
		requeueReason = "store is not ready yet"
		return shortRequeue, nil
	}

	if len(store.Status.QueueState.Transports) == 0 {
		log.Infow("Schedule short Reconcile, because worker queues are not known yet",
			zap.String("queue-error", store.Status.QueueState.Error))
		requeueReason = "worker queues are not known yet"
		return shortRequeue, nil
	}

	if store.Status.WorkerState.State == v1.DeploymentStateNotFound {
		log.Infow("Schedule short Reconcile, because worker deployments are not created yet",
			zap.String("worker-message", store.Status.WorkerState.Message))
		requeueReason = "worker deployments are not created yet"
		return shortRequeue, nil
	}

	log.Info("Reconcile finished, schedule long Reconcile")
	requeueReason = "store is ready, periodic long requeue"
	return longRequeue, nil
}
