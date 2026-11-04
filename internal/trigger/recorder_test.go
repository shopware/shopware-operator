package trigger_test

import (
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/shopware/shopware-operator/internal/tracing"
	"github.com/shopware/shopware-operator/internal/trigger"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/otel/attribute"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/sdk/trace/tracetest"
	appsv1 "k8s.io/api/apps/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/utils/ptr"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/event"
	"sigs.k8s.io/controller-runtime/pkg/predicate"
)

var storeKey = types.NamespacedName{Namespace: "shop", Name: "my-store"}

func ownedDeployment() *appsv1.Deployment {
	return &appsv1.Deployment{ObjectMeta: metav1.ObjectMeta{
		Name:            "my-store-storefront",
		Namespace:       "shop",
		ResourceVersion: "1",
		Generation:      1,
		Labels:          map[string]string{"app": "storefront"},
		Annotations:     map[string]string{"shopware.com/last-config-hash": "a"},
		OwnerReferences: []metav1.OwnerReference{{
			APIVersion: "shop.shopware.com/v1",
			Kind:       "Store",
			Name:       "my-store",
			Controller: ptr.To(true),
		}},
	}}
}

func alwaysTrue() predicate.Predicate {
	return predicate.NewPredicateFuncs(func(client.Object) bool { return true })
}

func TestPredicateRecordsOnlyAcceptedEvents(t *testing.T) {
	r := trigger.NewRecorder()
	deployment := ownedDeployment()

	rejecting := r.Predicate("Deployment", trigger.ControllerOwner("Store"), predicate.Funcs{
		UpdateFunc: func(event.UpdateEvent) bool { return false },
	})
	assert.False(t, rejecting.Update(event.UpdateEvent{ObjectOld: deployment, ObjectNew: deployment}))
	assertNoWatchEvents(t, r)

	accepting := r.Predicate("Deployment", trigger.ControllerOwner("Store"), alwaysTrue())
	assert.True(t, accepting.Create(event.CreateEvent{Object: deployment}))

	triggers := r.Drain(storeKey)
	require.Len(t, triggers, 1)
	assert.Equal(t, "Deployment", triggers[0].Source)
	assert.Equal(t, trigger.EventCreate, triggers[0].Event)
	assert.Equal(t, "my-store-storefront", triggers[0].Name)
	assert.False(t, triggers[0].At.IsZero())
	assertNoWatchEvents(t, r)
}

func TestPredicateKeys(t *testing.T) {
	r := trigger.NewRecorder()

	unowned := ownedDeployment()
	unowned.OwnerReferences = nil
	p := r.Predicate("Deployment", trigger.ControllerOwner("Store"), alwaysTrue())
	assert.True(t, p.Delete(event.DeleteEvent{Object: unowned}))
	assertNoWatchEvents(t, r)

	notController := ownedDeployment()
	notController.OwnerReferences[0].Controller = nil
	assert.True(t, p.Delete(event.DeleteEvent{Object: notController}))
	assertNoWatchEvents(t, r)

	store := &metav1.PartialObjectMetadata{ObjectMeta: metav1.ObjectMeta{Name: "my-store", Namespace: "shop"}}
	self := r.Predicate("Store", trigger.Self, alwaysTrue())
	assert.True(t, self.Delete(event.DeleteEvent{Object: store}))
	triggers := r.Drain(storeKey)
	require.Len(t, triggers, 1)
	assert.Equal(t, trigger.EventDelete, triggers[0].Event)
}

func TestRecorderCapsPendingTriggers(t *testing.T) {
	r := trigger.NewRecorder()
	for i := range 25 {
		r.Add(storeKey, trigger.Trigger{Name: fmt.Sprint(i)})
	}

	triggers := r.Drain(storeKey)
	require.Len(t, triggers, 20)
	assert.Equal(t, "5", triggers[0].Name)
	assert.Equal(t, "24", triggers[19].Name)
}

func TestNilRecorderIsSafe(t *testing.T) {
	var r *trigger.Recorder
	r.Add(storeKey, trigger.Trigger{})
	r.Finish(storeKey, "reason", ctrl.Result{RequeueAfter: time.Second}, nil)
	assert.Nil(t, r.Drain(storeKey))
}

func assertNoWatchEvents(t *testing.T, r *trigger.Recorder) {
	t.Helper()
	triggers := r.Drain(storeKey)
	require.Len(t, triggers, 1)
	assert.NotEqual(t, "watch event", trigger.Cause(triggers))
}

func TestDrainWithoutEvents(t *testing.T) {
	r := trigger.NewRecorder()

	triggers := r.Drain(storeKey)
	require.Len(t, triggers, 1)
	assert.Equal(t, "operator start", trigger.Cause(triggers))
	assert.Equal(t, "first reconcile since operator start", triggers[0].Reason)

	r.Finish(storeKey, "store is ready, periodic long requeue", ctrl.Result{RequeueAfter: 15 * time.Minute}, nil)
	triggers = r.Drain(storeKey)
	require.Len(t, triggers, 1)
	assert.Equal(t, "requeue", trigger.Cause(triggers))
	assert.Equal(t, trigger.SourceRequeue, triggers[0].Source)
	assert.Equal(t, "store is ready, periodic long requeue", triggers[0].Reason)
	assert.Equal(t, []string{"requeue_after:15m0s"}, triggers[0].Changes)
	assert.True(t, triggers[0].At.After(time.Now().Add(14*time.Minute)))

	triggers = r.Drain(storeKey)
	require.Len(t, triggers, 1)
	assert.Equal(t, "periodic resync", trigger.Cause(triggers))

	r.Finish(storeKey, "reconciling resources failed, no requeue", ctrl.Result{}, nil)
	assert.Equal(t, "periodic resync", trigger.Cause(r.Drain(storeKey)))

	r.Finish(storeKey, "ignored", ctrl.Result{}, errors.New("boom"))
	triggers = r.Drain(storeKey)
	require.Len(t, triggers, 1)
	assert.Equal(t, "requeue", trigger.Cause(triggers))
	assert.Equal(t, "previous reconcile returned an error, rate limited retry", triggers[0].Reason)
}

func TestWatchEventsTakePrecedenceOverRequeue(t *testing.T) {
	r := trigger.NewRecorder()
	r.Finish(storeKey, "store is not ready yet", ctrl.Result{RequeueAfter: 5 * time.Second}, nil)
	r.Add(storeKey, trigger.Trigger{Source: "CronJob", Event: trigger.EventUpdate, Name: "jobs"})

	triggers := r.Drain(storeKey)
	require.Len(t, triggers, 1)
	assert.Equal(t, "watch event", trigger.Cause(triggers))
	assert.Equal(t, "CronJob", triggers[0].Source)
}

func TestForgetClearsAllStateForKey(t *testing.T) {
	r := trigger.NewRecorder()
	r.Drain(storeKey)
	r.Add(storeKey, trigger.Trigger{Source: "CronJob", Event: trigger.EventUpdate, Name: "jobs"})
	r.Finish(storeKey, "store is not ready yet", ctrl.Result{RequeueAfter: 5 * time.Second}, nil)

	r.Forget(storeKey)

	triggers := r.Drain(storeKey)
	require.Len(t, triggers, 1)
	assert.Equal(t, "operator start", trigger.Cause(triggers))
	assert.Equal(t, "first reconcile since operator start", triggers[0].Reason)
}

func TestForgetOnNilRecorderIsSafe(t *testing.T) {
	var r *trigger.Recorder
	r.Forget(storeKey)
}

func TestCauseWithoutTriggers(t *testing.T) {
	assert.Equal(t, "unknown", trigger.Cause(nil))
}

func TestRecordOnSpan(t *testing.T) {
	recorder := tracetest.NewSpanRecorder()
	tracer := sdktrace.NewTracerProvider(sdktrace.WithSpanProcessor(recorder)).Tracer("test")

	_, requeueSpan := tracer.Start(t.Context(), "requeue")
	trigger.RecordOnSpan(requeueSpan, []trigger.Trigger{{
		Source:  trigger.SourceRequeue,
		Event:   trigger.EventRequeue,
		Name:    "my-store",
		Reason:  "store is ready, periodic long requeue",
		Changes: []string{"requeue_after:15m0s"},
	}})
	requeueSpan.End()

	_, eventSpan := tracer.Start(t.Context(), "event")
	trigger.RecordOnSpan(eventSpan, []trigger.Trigger{{
		Source:  "Deployment",
		Event:   trigger.EventUpdate,
		Name:    "my-store-storefront",
		Reason:  "only status or resourceVersion changed",
		Changes: []string{"resourceVersion:1->2"},
	}})
	eventSpan.End()

	spans := recorder.Ended()
	require.Len(t, spans, 2)

	assert.Contains(t, spans[0].Attributes(), tracing.AttrTriggerCause.String("requeue"))
	assert.Contains(t, spans[0].Attributes(), tracing.AttrTriggers.StringSlice([]string{
		"Requeue/my-store requeue: store is ready, periodic long requeue (requeue_after:15m0s)",
	}))
	require.Len(t, spans[0].Events(), 1)

	assert.Contains(t, spans[1].Attributes(), tracing.AttrTriggerCause.String("watch event"))
	assert.Contains(t, spans[1].Attributes(), tracing.AttrTriggerCount.Int(1))
	assert.Contains(t, spans[1].Attributes(), tracing.AttrTriggerSources.String("Deployment"))
	assert.Contains(t, spans[1].Attributes(), tracing.AttrTriggers.StringSlice([]string{
		"Deployment/my-store-storefront update: only status or resourceVersion changed (resourceVersion:1->2)",
	}))
	require.Len(t, spans[1].Events(), 1)
	attrs := spans[1].Events()[0].Attributes
	assert.Equal(t, tracing.EventReconcileTrigger, spans[1].Events()[0].Name)
	assert.Contains(t, attrs, tracing.AttrTriggerSource.String("Deployment"))
	assert.Contains(t, attrs, tracing.AttrTriggerReason.String("only status or resourceVersion changed"))
	assert.Contains(t, attrs, attribute.StringSlice(string(tracing.AttrTriggerChanges), []string{"resourceVersion:1->2"}))
}

func TestSourcesAreSortedAndUnique(t *testing.T) {
	assert.Equal(t, []string{"CronJob", "Deployment"}, trigger.Sources([]trigger.Trigger{
		{Source: "Deployment"}, {Source: "CronJob"}, {Source: "Deployment"},
	}))
}

func TestTriggerString(t *testing.T) {
	assert.Equal(t,
		"Deployment/web update: labels or annotations changed (label:extra)",
		trigger.Trigger{Source: "Deployment", Name: "web", Event: trigger.EventUpdate, Reason: "labels or annotations changed", Changes: []string{"label:extra"}}.String(),
	)
}
