package controller

import (
	"context"
	"errors"
	"fmt"
	"time"

	"go.uber.org/zap"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/tools/record"
	k8sretry "k8s.io/client-go/util/retry"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"

	shopv1 "github.com/shopware/shopware-operator/api/v1"
	"github.com/shopware/shopware-operator/internal/k8s"
	"github.com/shopware/shopware-operator/internal/logging"
	"github.com/shopware/shopware-operator/internal/pod"
	"github.com/shopware/shopware-operator/internal/service"
	"github.com/shopware/shopware-operator/internal/tracing"
)

// TerminalDebugInstanceReconciler reconciles a TerminalDebugInstance object
type TerminalDebugInstanceReconciler struct {
	client.Client
	Scheme   *runtime.Scheme
	Recorder record.EventRecorder
	Logger   *zap.SugaredLogger
}

// +kubebuilder:rbac:groups=shop.shopware.com,namespace=default,resources=terminaldebuginstances,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=shop.shopware.com,namespace=default,resources=terminaldebuginstances/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=shop.shopware.com,namespace=default,resources=terminaldebuginstances/finalizers,verbs=update
// +kubebuilder:rbac:groups="",namespace=default,resources=pods,verbs=get;list;watch;create;delete;
// +kubebuilder:rbac:groups="",namespace=default,resources=services,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups="apps",namespace=default,resources=replicasets,verbs=get;list;watch

func (r *TerminalDebugInstanceReconciler) Reconcile(ctx context.Context, req ctrl.Request) (rr ctrl.Result, err error) {
	ctx, span := tracing.StartReconcile(ctx, "TerminalDebugInstance", req)
	defer func() { tracing.EndReconcile(span, rr, &err) }()

	log := r.Logger.
		With(zap.String("namespace", req.Namespace)).
		With(zap.String("name", req.Name))
	ctx = logging.WithLogger(ctx, log)

	instance := &shopv1.TerminalDebugInstance{}
	if err := r.Get(ctx, req.NamespacedName, instance); err != nil {
		if k8serrors.IsNotFound(err) {
			return ctrl.Result{}, nil
		}
		return ctrl.Result{}, fmt.Errorf("get terminal debug instance: %w", err)
	}
	if instance.DeletionTimestamp != nil {
		return ctrl.Result{}, nil
	}

	if instance.IsState(shopv1.StoreDebugInstanceStateDone, shopv1.StoreDebugInstanceStateError) {
		return r.cleanup(ctx, instance)
	}

	status := r.reconcileDebugPod(ctx, instance)
	if err := r.reconcileService(ctx, instance); err != nil {
		log.Errorw("failed to reconcile service", zap.Error(err))
		tracing.RecordError(span, err)
	}
	if err := writeTerminalDebugInstanceStatus(ctx, r.Client, req.NamespacedName, status); err != nil {
		log.Errorw("failed to update status", zap.Error(err))
		tracing.RecordError(span, err)
		return ctrl.Result{RequeueAfter: 10 * time.Second}, nil
	}

	return ctrl.Result{RequeueAfter: 20 * time.Second}, nil
}

// SetupWithManager sets up the controller with the Manager.
func (r *TerminalDebugInstanceReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&shopv1.TerminalDebugInstance{}).
		Owns(&corev1.Pod{}).
		Owns(&corev1.Service{}).
		Named("terminaldebuginstance").
		Complete(r)
}

func (r *TerminalDebugInstanceReconciler) reconcileService(ctx context.Context, instance *shopv1.TerminalDebugInstance) error {
	if len(instance.Spec.ExtraContainerPorts) == 0 {
		return nil
	}
	svc := service.TerminalDebugService(*instance)

	changed, err := k8s.HasObjectChanged(ctx, r.Client, svc)
	if err != nil {
		return fmt.Errorf("reconcile debug service: %w", err)
	}
	if !changed {
		return nil
	}

	r.Recorder.Event(instance, "Normal", "Debug service update",
		fmt.Sprintf("Update debug service %s in namespace %s", svc.Name, svc.Namespace))
	if err := k8s.EnsureService(ctx, r.Client, instance, svc, r.Scheme, true); err != nil {
		return fmt.Errorf("reconcile debug service: %w", err)
	}
	return nil
}

// reconcileDebugPod creates the debug pod when it is missing and derives the
// next status from it.
func (r *TerminalDebugInstanceReconciler) reconcileDebugPod(
	ctx context.Context,
	instance *shopv1.TerminalDebugInstance,
) shopv1.TerminalDebugInstanceStatus {
	status := *instance.Status.DeepCopy()
	setState := func(state shopv1.StoreDebugInstanceState, message, reason, conditionStatus string) {
		status.State = state
		status.AddCondition(shopv1.StoreDebugInstanceCondition{
			Type:           state,
			LastUpdateTime: metav1.Now(),
			Message:        message,
			Reason:         reason,
			Status:         conditionStatus,
		})
	}

	if terminalDebugInstanceExpired(instance) {
		setState(shopv1.StoreDebugInstanceStateDone, "Terminal debug instance expired", "", "True")
		return status
	}

	existing := &corev1.Pod{}
	err := r.Get(ctx, types.NamespacedName{Namespace: instance.Namespace, Name: instance.Name}, existing)
	if err == nil {
		// A pod with our name that we do not control is an unrelated pod. Never
		// report it as the debug pod. The error state makes cleanup run, which
		// only deletes objects controlled by this instance.
		if !metav1.IsControlledBy(existing, instance) {
			status.PodName = ""
			status.Container = ""
			setState(shopv1.StoreDebugInstanceStateError,
				fmt.Sprintf("Pod %s already exists and is not controlled by this TerminalDebugInstance, choose a different name", existing.Name),
				"PodNameCollision", Error)
			return status
		}
		status.PodName = existing.Name
		state, message := terminalDebugStateFromPod(existing)
		conditionStatus := "True"
		if state == shopv1.StoreDebugInstanceStateError {
			conditionStatus = Error
		}
		setState(state, message, "", conditionStatus)
		return status
	}
	if !k8serrors.IsNotFound(err) {
		setState(shopv1.StoreDebugInstanceStatePending, err.Error(), "ReconcileError", Error)
		return status
	}

	debugPod, container, err := r.buildDebugPod(ctx, instance)
	switch {
	case errors.Is(err, pod.ErrNoSourcePod):
		setState(shopv1.StoreDebugInstanceStateWait, err.Error(), "NoSourcePod", Error)
		return status
	case errors.Is(err, pod.ErrMultipleWorkloads):
		setState(shopv1.StoreDebugInstanceStateError, err.Error(), "MultipleWorkloads", Error)
		return status
	case err != nil:
		setState(shopv1.StoreDebugInstanceStateError, err.Error(), "InvalidSource", Error)
		return status
	}

	r.Recorder.Event(instance, "Normal", "Create debug pod",
		fmt.Sprintf("Creating debug pod %s in namespace %s", debugPod.Name, debugPod.Namespace))
	if err := k8s.EnsurePod(ctx, r.Client, instance, debugPod, r.Scheme, true); err != nil {
		setState(shopv1.StoreDebugInstanceStatePending, err.Error(), "ReconcileError", Error)
		return status
	}

	status.PodName = debugPod.Name
	status.Container = container
	setState(shopv1.StoreDebugInstanceStatePending, "Debug pod created, waiting for it to start", "", "True")
	return status
}

func (r *TerminalDebugInstanceReconciler) buildDebugPod(
	ctx context.Context,
	instance *shopv1.TerminalDebugInstance,
) (*corev1.Pod, string, error) {
	selector, err := metav1.LabelSelectorAsSelector(&instance.Spec.PodSelector)
	if err != nil {
		return nil, "", fmt.Errorf("parse pod selector: %w", err)
	}
	if selector.Empty() {
		return nil, "", errors.New("pod selector must not be empty")
	}

	pods := &corev1.PodList{}
	if err := r.List(ctx, pods, client.InNamespace(instance.Namespace), client.MatchingLabelsSelector{Selector: selector}); err != nil {
		return nil, "", fmt.Errorf("list source pods: %w", err)
	}
	replicaSets := &appsv1.ReplicaSetList{}
	if err := r.List(ctx, replicaSets, client.InNamespace(instance.Namespace)); err != nil {
		return nil, "", fmt.Errorf("list replica sets: %w", err)
	}
	source, err := pod.SelectTerminalDebugSource(pods.Items, replicaSets.Items)
	if err != nil {
		return nil, "", err
	}

	services := &corev1.ServiceList{}
	if err := r.List(ctx, services, client.InNamespace(instance.Namespace)); err != nil {
		return nil, "", fmt.Errorf("list services: %w", err)
	}

	return pod.TerminalDebugPod(*instance, source, pod.ServiceSelectorKeys(services.Items))
}

// cleanup deletes the debug pod of a finished instance, and the instance once
// its duration has passed. Deleting the instance also removes the pod it owns.
func (r *TerminalDebugInstanceReconciler) cleanup(
	ctx context.Context,
	instance *shopv1.TerminalDebugInstance,
) (ctrl.Result, error) {
	debugPod := &corev1.Pod{ObjectMeta: metav1.ObjectMeta{Namespace: instance.Namespace, Name: instance.Name}}
	// Only delete objects controlled by this instance (matched by UID). A name
	// collision with an unrelated pod or service must never delete that object.
	if _, err := k8s.DeleteControlledObject(ctx, r.Client, instance, debugPod); err != nil {
		return ctrl.Result{}, fmt.Errorf("delete debug pod: %w", err)
	}
	svc := service.TerminalDebugService(*instance)
	if _, err := k8s.DeleteControlledObject(ctx, r.Client, instance, svc); err != nil {
		return ctrl.Result{}, fmt.Errorf("delete debug service: %w", err)
	}

	duration := instance.Spec.Duration.Duration
	if duration <= 0 {
		return ctrl.Result{}, nil
	}
	if remaining := time.Until(instance.CreationTimestamp.Add(duration)); remaining > 0 {
		return ctrl.Result{RequeueAfter: remaining}, nil
	}

	if err := r.Delete(ctx, instance); err != nil && !k8serrors.IsNotFound(err) {
		return ctrl.Result{}, fmt.Errorf("delete terminal debug instance: %w", err)
	}
	return ctrl.Result{}, nil
}

func terminalDebugInstanceExpired(instance *shopv1.TerminalDebugInstance) bool {
	duration := instance.Spec.Duration.Duration
	return duration > 0 && time.Now().After(instance.CreationTimestamp.Add(duration))
}

func terminalDebugStateFromPod(p *corev1.Pod) (shopv1.StoreDebugInstanceState, string) {
	switch p.Status.Phase {
	case corev1.PodRunning:
		return shopv1.StoreDebugInstanceStateRunning, "Pod is running successfully"
	case corev1.PodFailed, corev1.PodSucceeded:
		return shopv1.StoreDebugInstanceStateError, fmt.Sprintf("Pod stopped in phase %s", p.Status.Phase)
	case corev1.PodPending:
		return shopv1.StoreDebugInstanceStatePending, "Pod is pending, waiting for resources"
	default:
		return shopv1.StoreDebugInstanceStatePending, fmt.Sprintf("Pod in unknown state: %s", p.Status.Phase)
	}
}

func writeTerminalDebugInstanceStatus(
	ctx context.Context,
	cl client.Client,
	nn types.NamespacedName,
	status shopv1.TerminalDebugInstanceStatus,
) error {
	return k8sretry.RetryOnConflict(k8sretry.DefaultRetry, func() error {
		cr := &shopv1.TerminalDebugInstance{}
		if err := cl.Get(ctx, nn, cr); err != nil {
			return fmt.Errorf("write status: %w", err)
		}
		cr.Status = status
		return cl.Status().Update(ctx, cr)
	})
}
