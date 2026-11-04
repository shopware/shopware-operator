package trigger

import (
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/shopware/shopware-operator/internal/k8s"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/event"
	"sigs.k8s.io/controller-runtime/pkg/predicate"
)

const maxPendingPerKey = 20

const (
	EventCreate  = "create"
	EventUpdate  = "update"
	EventDelete  = "delete"
	EventGeneric = "generic"
	EventMapped  = "mapped"
	EventRequeue = "requeue"
	EventResync  = "resync"
	EventStartup = "startup"
)

const (
	SourceRequeue    = "Requeue"
	SourceController = "Controller"
)

type scheduledRequeue struct {
	Reason string
	After  time.Duration
	At     time.Time
}

type Trigger struct {
	Source  string
	Event   string
	Name    string
	Reason  string
	Changes []string
	At      time.Time
}

func (t Trigger) String() string {
	summary := fmt.Sprintf("%s/%s %s: %s", t.Source, t.Name, t.Event, t.Reason)
	if len(t.Changes) == 0 {
		return summary
	}
	return summary + " (" + strings.Join(t.Changes, ", ") + ")"
}

type KeyFunc func(client.Object) (types.NamespacedName, bool)

func Self(obj client.Object) (types.NamespacedName, bool) {
	return types.NamespacedName{Namespace: obj.GetNamespace(), Name: obj.GetName()}, true
}

func ControllerOwner(kind string) KeyFunc {
	return func(obj client.Object) (types.NamespacedName, bool) {
		for _, ref := range obj.GetOwnerReferences() {
			if ref.Controller != nil && *ref.Controller && ref.Kind == kind {
				return types.NamespacedName{Namespace: obj.GetNamespace(), Name: ref.Name}, true
			}
		}
		return types.NamespacedName{}, false
	}
}

type Recorder struct {
	mu       sync.Mutex
	pending  map[types.NamespacedName][]Trigger
	requeues map[types.NamespacedName]scheduledRequeue
	seen     map[types.NamespacedName]struct{}
}

func NewRecorder() *Recorder {
	return &Recorder{
		pending:  make(map[types.NamespacedName][]Trigger),
		requeues: make(map[types.NamespacedName]scheduledRequeue),
		seen:     make(map[types.NamespacedName]struct{}),
	}
}

func (r *Recorder) Add(key types.NamespacedName, t Trigger) {
	if r == nil {
		return
	}
	if t.At.IsZero() {
		t.At = time.Now()
	}

	r.mu.Lock()
	defer r.mu.Unlock()

	pending := append(r.pending[key], t)
	if len(pending) > maxPendingPerKey {
		pending = pending[len(pending)-maxPendingPerKey:]
	}
	r.pending[key] = pending
}

func (r *Recorder) Drain(key types.NamespacedName) []Trigger {
	if r == nil {
		return nil
	}

	r.mu.Lock()
	defer r.mu.Unlock()

	pending := r.pending[key]
	delete(r.pending, key)

	requeue, scheduled := r.requeues[key]
	delete(r.requeues, key)

	_, seen := r.seen[key]
	r.seen[key] = struct{}{}

	if len(pending) != 0 {
		return pending
	}

	switch {
	case scheduled:
		return []Trigger{{
			Source:  SourceRequeue,
			Event:   EventRequeue,
			Name:    key.Name,
			Reason:  requeue.Reason,
			Changes: []string{"requeue_after:" + requeue.After.String()},
			At:      requeue.At.Add(requeue.After),
		}}
	case !seen:
		return []Trigger{{Source: SourceController, Event: EventStartup, Name: key.Name, Reason: "first reconcile since operator start", At: time.Now()}}
	default:
		return []Trigger{{Source: SourceController, Event: EventResync, Name: key.Name, Reason: "no watch event and no requeue scheduled, periodic cache resync", At: time.Now()}}
	}
}

func (r *Recorder) Forget(key types.NamespacedName) {
	if r == nil {
		return
	}

	r.mu.Lock()
	defer r.mu.Unlock()

	delete(r.pending, key)
	delete(r.requeues, key)
	delete(r.seen, key)
}

func (r *Recorder) Finish(key types.NamespacedName, reason string, result ctrl.Result, err error) {
	if r == nil {
		return
	}

	r.mu.Lock()
	defer r.mu.Unlock()

	now := time.Now()
	switch {
	case err != nil:
		r.requeues[key] = scheduledRequeue{Reason: "previous reconcile returned an error, rate limited retry", At: now}
	case result.RequeueAfter > 0:
		r.requeues[key] = scheduledRequeue{Reason: reason, After: result.RequeueAfter, At: now}
	default:
		delete(r.requeues, key)
	}
}

func (r *Recorder) Predicate(source string, key KeyFunc, p predicate.Predicate) predicate.Predicate {
	return predicate.Funcs{
		CreateFunc: func(e event.CreateEvent) bool {
			return r.record(p.Create(e), source, EventCreate, key, e.Object, Trigger{Reason: "object created"})
		},
		UpdateFunc: func(e event.UpdateEvent) bool {
			if !p.Update(e) {
				return false
			}
			reason, changes := k8s.DescribeUpdate(e.ObjectOld, e.ObjectNew)
			return r.record(true, source, EventUpdate, key, e.ObjectNew, Trigger{Reason: reason, Changes: changes})
		},
		DeleteFunc: func(e event.DeleteEvent) bool {
			return r.record(p.Delete(e), source, EventDelete, key, e.Object, Trigger{Reason: "object deleted"})
		},
		GenericFunc: func(e event.GenericEvent) bool {
			return r.record(p.Generic(e), source, EventGeneric, key, e.Object, Trigger{Reason: "generic event"})
		},
	}
}

func (r *Recorder) record(
	accepted bool,
	source string,
	eventType string,
	key KeyFunc,
	obj client.Object,
	t Trigger,
) bool {
	if !accepted || obj == nil {
		return accepted
	}
	nn, ok := key(obj)
	if !ok {
		return accepted
	}
	t.Source = source
	t.Event = eventType
	t.Name = obj.GetName()
	r.Add(nn, t)
	return accepted
}
