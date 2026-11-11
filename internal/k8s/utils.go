/*
Copyright 2018 Percona, LLC
Copyright 2024 shopware AG

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

   http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.



*/

package k8s

import (
	"context"
	"crypto/md5"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"maps"
	"os"
	"reflect"
	"slices"
	"strings"
	"time"

	cm "github.com/cert-manager/cert-manager/pkg/apis/certmanager/v1"
	kedav1alpha1 "github.com/kedacore/keda/v2/apis/keda/v1alpha1"
	"github.com/pkg/errors"
	v1 "github.com/shopware/shopware-operator/api/v1"
	"github.com/shopware/shopware-operator/internal/logging"
	"github.com/shopware/shopware-operator/internal/tracing"
	"github.com/shopware/shopware-operator/internal/util"
	"go.uber.org/zap"
	appsv1 "k8s.io/api/apps/v1"
	autoscalingv2 "k8s.io/api/autoscaling/v2"
	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	networkingv1 "k8s.io/api/networking/v1"
	policy "k8s.io/api/policy/v1"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/event"
	"sigs.k8s.io/controller-runtime/pkg/predicate"
	gatewayv1 "sigs.k8s.io/gateway-api/apis/v1"
)

var ignoredAnnotations = []string{"deployment.kubernetes.io/revision"}

func GetWatchNamespace() (string, error) {
	ns, found := os.LookupEnv("NAMESPACE")
	if !found {
		return "", fmt.Errorf("%s must be set", "NAMESPACE")
	}
	return ns, nil
}

func GetOperatorNamespace() (string, error) {
	ns, found := os.LookupEnv("OPERATOR_NAMESPACE")
	if found {
		return ns, nil
	}

	nsBytes, err := os.ReadFile("/var/run/secrets/kubernetes.io/serviceaccount/namespace")
	if err == nil {
		return strings.TrimSpace(string(nsBytes)), nil
	}

	return "", fmt.Errorf(
		"either set the namespace via env `%s` or run the operator as a pod",
		"OPERATOR_NAMESPACE",
	)
}

func GetStore(
	ctx context.Context,
	cl client.Client,
	nn types.NamespacedName,
) (*v1.Store, error) {
	cr := new(v1.Store)
	if err := cl.Get(ctx, nn, cr); err != nil {
		return nil, errors.Wrapf(err, "get %v", nn.String())
	}

	return cr, nil
}

func GetStoreExec(
	ctx context.Context,
	cl client.Client,
	nn types.NamespacedName,
) (*v1.StoreExec, error) {
	cr := new(v1.StoreExec)
	if err := cl.Get(ctx, nn, cr); err != nil {
		return nil, errors.Wrapf(err, "get %v", nn.String())
	}

	return cr, nil
}

func GetStoreSnapshotCreate(
	ctx context.Context,
	cl client.Client,
	nn types.NamespacedName,
) (*v1.StoreSnapshotCreate, error) {
	cr := new(v1.StoreSnapshotCreate)
	if err := cl.Get(ctx, nn, cr); err != nil {
		return nil, errors.Wrapf(err, "get %v", nn.String())
	}

	return cr, nil
}

func GetStoreSnapshotRestore(
	ctx context.Context,
	cl client.Client,
	nn types.NamespacedName,
) (*v1.StoreSnapshotRestore, error) {
	cr := new(v1.StoreSnapshotRestore)
	if err := cl.Get(ctx, nn, cr); err != nil {
		return nil, errors.Wrapf(err, "get %v", nn.String())
	}

	return cr, nil
}

func GetSecret(
	ctx context.Context,
	cl client.Client,
	nn types.NamespacedName,
) (*corev1.Secret, error) {
	cr := new(corev1.Secret)
	if err := cl.Get(ctx, nn, cr); err != nil {
		return nil, errors.Wrapf(err, "get %v", nn.String())
	}

	return cr, nil
}

func GetStoreDebugInstance(
	ctx context.Context,
	cl client.Client,
	nn types.NamespacedName,
) (*v1.StoreDebugInstance, error) {
	cr := new(v1.StoreDebugInstance)
	if err := cl.Get(ctx, nn, cr); err != nil {
		return nil, errors.Wrapf(err, "get %v", nn.String())
	}

	return cr, nil
}

func ObjectExists(
	ctx context.Context,
	cl client.Reader,
	nn types.NamespacedName,
	o client.Object,
) (bool, error) {
	if err := cl.Get(ctx, nn, o); err != nil {
		if k8serrors.IsNotFound(err) {
			return false, nil
		}

		return false, err
	}

	return true, nil
}

// ErrNotControlledByOwner is returned when an existing object has a name we
// want to use but is not controlled by the given owner.
var ErrNotControlledByOwner = errors.New("object is not controlled by owner")

// DeleteControlledObject deletes the object with the name and namespace of obj,
// but only if it is controlled by owner. An object that has the same name but a
// different controller is left alone. The delete carries a UID precondition, so
// an object that was replaced after the ownership check is not deleted either.
// It reports whether an object was deleted.
func DeleteControlledObject(
	ctx context.Context,
	cl client.Client,
	owner metav1.Object,
	obj client.Object,
) (bool, error) {
	if owner == nil || owner.GetUID() == "" {
		return false, errors.New("owner must have a UID")
	}

	val := reflect.Indirect(reflect.ValueOf(obj))
	existing := reflect.New(val.Type()).Interface().(client.Object)

	nn := types.NamespacedName{Name: obj.GetName(), Namespace: obj.GetNamespace()}
	if err := cl.Get(ctx, nn, existing); err != nil {
		if k8serrors.IsNotFound(err) {
			return false, nil
		}
		return false, errors.Wrapf(err, "get %v", nn.String())
	}

	if !metav1.IsControlledBy(existing, owner) {
		logging.FromContext(ctx).Infow("Skip delete, object is not controlled by owner",
			zap.String("kind", objectKind(obj)),
			zap.String("obj-name", nn.String()),
			zap.String("owner", owner.GetName()))
		return false, nil
	}

	uid := existing.GetUID()
	if err := cl.Delete(ctx, existing, client.Preconditions{UID: &uid}); err != nil {
		// A failed precondition (conflict) means the object was replaced since
		// we looked at it, so it is no longer ours to delete.
		if k8serrors.IsNotFound(err) || k8serrors.IsConflict(err) {
			return false, nil
		}
		return false, errors.Wrapf(err, "delete %v", nn.String())
	}

	return true, nil
}

// EnsureService creates or updates the Service. An existing Service that is not
// controlled by owner is rejected, and the update is rejected with a conflict
// if the Service changes between the ownership check and the patch. Both are
// enforced in EnsureObjectWithHash, on the same object that gets patched.
func EnsureService(
	ctx context.Context,
	cl client.Client,
	owner metav1.Object,
	svc *corev1.Service,
	s *runtime.Scheme,
	saveOldMeta bool,
) error {
	if owner == nil {
		return errors.New("service owner must not be nil")
	}

	return EnsureObjectWithHash(ctx, cl, owner, svc, s)
}

func EnsureHPA(
	ctx context.Context,
	cl client.Client,
	owner metav1.Object,
	svc *autoscalingv2.HorizontalPodAutoscaler,
	s *runtime.Scheme,
	saveOldMeta bool,
) error {
	oldSvc := new(corev1.Service)
	err := cl.Get(ctx, types.NamespacedName{
		Name:      svc.GetName(),
		Namespace: svc.GetNamespace(),
	}, oldSvc)
	if err != nil {
		if k8serrors.IsNotFound(err) {
			return EnsureObjectWithHash(ctx, cl, owner, svc, s)
		}
		return errors.Wrap(err, "get object")
	}

	return EnsureObjectWithHash(ctx, cl, owner, svc, s)
}

func EnsureCronJob(
	ctx context.Context,
	cl client.Client,
	owner metav1.Object,
	job *batchv1.CronJob,
	s *runtime.Scheme,
	saveOldMeta bool,
) error {
	oldJob := new(batchv1.Job)
	err := cl.Get(ctx, types.NamespacedName{
		Name:      job.GetName(),
		Namespace: job.GetNamespace(),
	}, oldJob)
	if err != nil {
		if k8serrors.IsNotFound(err) {
			return EnsureObjectWithHash(ctx, cl, owner, job, s)
		}
		return errors.Wrap(err, "get object")
	}

	return EnsureObjectWithHash(ctx, cl, owner, job, s)
}

func EnsurePod(
	ctx context.Context,
	cl client.Client,
	owner metav1.Object,
	pod *corev1.Pod,
	s *runtime.Scheme,
	saveOldMeta bool,
) error {
	oldPod := new(corev1.Pod)
	err := cl.Get(ctx, types.NamespacedName{
		Name:      pod.GetName(),
		Namespace: pod.GetNamespace(),
	}, oldPod)
	if err != nil {
		if k8serrors.IsNotFound(err) {
			return EnsureObjectWithHash(ctx, cl, owner, pod, s)
		}
		return errors.Wrap(err, "get object")
	}

	return EnsureObjectWithHash(ctx, cl, owner, pod, s)
}

func EnsureJob(
	ctx context.Context,
	cl client.Client,
	owner metav1.Object,
	job *batchv1.Job,
	s *runtime.Scheme,
	saveOldMeta bool,
) error {
	oldJob := new(batchv1.Job)
	err := cl.Get(ctx, types.NamespacedName{
		Name:      job.GetName(),
		Namespace: job.GetNamespace(),
	}, oldJob)
	if err != nil {
		if k8serrors.IsNotFound(err) {
			return EnsureObjectWithHash(ctx, cl, owner, job, s)
		}
		return errors.Wrap(err, "get object")
	}

	return EnsureObjectWithHash(ctx, cl, owner, job, s)
}

func EnsurePDB(
	ctx context.Context,
	cl client.Client,
	owner metav1.Object,
	pdb *policy.PodDisruptionBudget,
	s *runtime.Scheme,
	saveOldMeta bool,
) error {
	oldDep := new(policy.PodDisruptionBudget)
	err := cl.Get(ctx, types.NamespacedName{
		Name:      pdb.GetName(),
		Namespace: pdb.GetNamespace(),
	}, oldDep)
	if err != nil {
		if k8serrors.IsNotFound(err) {
			return EnsureObjectWithHash(ctx, cl, owner, pdb, s)
		}
		return errors.Wrap(err, "get object")
	}

	return EnsureObjectWithHash(ctx, cl, owner, pdb, s)
}

func EnsureIngress(
	ctx context.Context,
	cl client.Client,
	owner metav1.Object,
	ingress *networkingv1.Ingress,
	s *runtime.Scheme,
	saveOldMeta bool,
) error {
	oldDep := new(appsv1.Deployment)
	err := cl.Get(ctx, types.NamespacedName{
		Name:      ingress.GetName(),
		Namespace: ingress.GetNamespace(),
	}, oldDep)
	if err != nil {
		if k8serrors.IsNotFound(err) {
			return EnsureObjectWithHash(ctx, cl, owner, ingress, s)
		}
		return errors.Wrap(err, "get object")
	}

	return EnsureObjectWithHash(ctx, cl, owner, ingress, s)
}

func EnsureHTTPRoute(
	ctx context.Context,
	cl client.Client,
	owner metav1.Object,
	httpRoute *gatewayv1.HTTPRoute,
	s *runtime.Scheme,
	saveOldMeta bool,
) error {
	oldRoute := new(gatewayv1.HTTPRoute)
	err := cl.Get(ctx, types.NamespacedName{
		Name:      httpRoute.GetName(),
		Namespace: httpRoute.GetNamespace(),
	}, oldRoute)
	if err != nil {
		if k8serrors.IsNotFound(err) {
			return EnsureObjectWithHash(ctx, cl, owner, httpRoute, s)
		}
		return errors.Wrap(err, "get object")
	}

	return EnsureObjectWithHash(ctx, cl, owner, httpRoute, s)
}

func EnsureDeployment(
	ctx context.Context,
	cl client.Client,
	owner metav1.Object,
	deployment *appsv1.Deployment,
	s *runtime.Scheme,
	saveOldMeta bool,
) error {
	oldDep := new(appsv1.Deployment)
	err := cl.Get(ctx, types.NamespacedName{
		Name:      deployment.GetName(),
		Namespace: deployment.GetNamespace(),
	}, oldDep)
	if err != nil {
		if k8serrors.IsNotFound(err) {
			return EnsureObjectWithHash(ctx, cl, owner, deployment, s)
		}
		return errors.Wrap(err, "get object")
	}

	return EnsureObjectWithHash(ctx, cl, owner, deployment, s)
}

const (
	changeReasonNotFound    = "object does not exist yet"
	changeReasonHashMissing = "deployed object has no config hash annotation"
	changeReasonHashChanged = "desired config differs from deployed config (config hash changed)"
	changeReasonMetadata    = "labels or annotations differ from deployed object"
)

type ObjectChange struct {
	Reason      string
	ChangedKeys []string
}

func (c ObjectChange) Changed() bool {
	return c.Reason != ""
}

// HasObjectChangedPredicate implements a default update predicate function where the operator would also change something
//
// This predicate will skip update events that have no change in the object's metadata.
// Delete events will always trigger a reconcile of the resource
func HasObjectChangedPredicate() predicate.Predicate {
	return predicate.Funcs{
		// Skip creates, because they come from the operator so we don't want to trigger a reconcile on them.
		CreateFunc: func(e event.CreateEvent) bool {
			return false
		},
		UpdateFunc: func(e event.UpdateEvent) bool {
			oldObject, newObject := e.ObjectOld, e.ObjectNew
			if oldObject == nil || newObject == nil {
				return true
			}

			// Includes hash diff check
			return len(ChangedMetaKeys(oldObject, newObject)) != 0
		},
		// On delete Always run the operator owned object
		DeleteFunc: func(event.DeleteEvent) bool {
			return true
		},
		GenericFunc: func(event.GenericEvent) bool {
			return true
		},
	}
}

func HasObjectChanged(
	ctx context.Context,
	cl client.Client,
	obj client.Object,
) (bool, error) {
	change, err := DetectObjectChange(ctx, cl, obj)
	if err != nil {
		return true, err
	}
	tracing.RecordObjectChange(ctx, objectKind(obj), obj.GetName(), change.Reason, change.ChangedKeys)
	return change.Changed(), nil
}

func DetectObjectChange(
	ctx context.Context,
	cl client.Client,
	obj client.Object,
) (ObjectChange, error) {
	if obj.GetAnnotations() == nil {
		obj.SetAnnotations(make(map[string]string))
	}

	objAnnotations := obj.GetAnnotations()
	delete(objAnnotations, util.LastConfigHashAnnotation)
	obj.SetAnnotations(objAnnotations)

	val := reflect.ValueOf(obj)
	if val.Kind() == reflect.Pointer {
		val = reflect.Indirect(val)
	}
	oldObject := reflect.New(val.Type()).Interface().(client.Object)

	nn := types.NamespacedName{
		Name:      obj.GetName(),
		Namespace: obj.GetNamespace(),
	}
	if err := cl.Get(ctx, nn, oldObject); err != nil {
		if !k8serrors.IsNotFound(err) {
			return ObjectChange{}, errors.Wrapf(err, "get %v", nn.String())
		}
		return ObjectChange{Reason: changeReasonNotFound}, nil
	}

	hashChanged, err := objectHashChanged(oldObject, obj)
	if err != nil {
		return ObjectChange{}, errors.Wrap(err, "DetectObjectChange")
	}
	if hashChanged.Changed() {
		return hashChanged, nil
	}

	if secretHashChanged(oldObject, obj) {
		return ObjectChange{Reason: changeReasonSecretChanged}, nil
	}

	changedFields := ChangedMetaKeys(oldObject, obj)
	if len(changedFields) == 0 {
		return ObjectChange{}, nil
	}
	return ObjectChange{Reason: changeReasonMetadata, ChangedKeys: changedFields}, nil
}

func ChangedMetaKeys(old, new metav1.Object) []string {
	keys := diffKeys("label", old.GetLabels(), new.GetLabels())
	return append(keys, diffKeys("annotation", filteredAnnotations(old), filteredAnnotations(new))...)
}

func filteredAnnotations(obj metav1.Object) map[string]string {
	annotations := maps.Clone(obj.GetAnnotations())
	delete(annotations, util.LastConfigHashAnnotation)
	for _, key := range ignoredAnnotations {
		delete(annotations, key)
	}
	return annotations
}

func objectHashChanged(old, new client.Object) (ObjectChange, error) {
	hash, err := ObjectHash(new)
	if err != nil {
		return ObjectChange{}, errors.Wrap(err, "calculate object hash")
	}

	oldHash, ok := old.GetAnnotations()[util.LastConfigHashAnnotation]
	if !ok {
		return ObjectChange{Reason: changeReasonHashMissing}, nil
	}
	if oldHash != hash {
		return ObjectChange{Reason: changeReasonHashChanged}, nil
	}
	return ObjectChange{}, nil
}

func diffKeys(prefix string, old, new map[string]string) []string {
	var keys []string
	for k, v := range new {
		if oldValue, ok := old[k]; !ok || oldValue != v {
			keys = append(keys, prefix+":"+k)
		}
	}
	for k := range old {
		if _, ok := new[k]; !ok {
			keys = append(keys, prefix+":"+k)
		}
	}
	slices.Sort(keys)
	return keys
}

func objectKind(obj client.Object) string {
	if kind := obj.GetObjectKind().GroupVersionKind().Kind; kind != "" {
		return kind
	}
	return reflect.Indirect(reflect.ValueOf(obj)).Type().Name()
}

func logObjectChange(
	ctx context.Context,
	oldObject client.Object,
	obj client.Object,
	hash string,
	hashChanged bool,
	secretChanged bool,
	objectMetaChangedFields []string,
) {
	if secretChanged && !hashChanged {
		logging.FromContext(ctx).Infow("Store secret hash has changed",
			zap.String("kind", obj.GetObjectKind().GroupVersionKind().Kind),
			zap.String("obj-name", obj.GetName()),
			zap.String("old-secret-hash", secretHashOf(oldObject)),
			zap.String("new-secret-hash", secretHashOf(obj)))
	} else if hashChanged {
		diff, err := ObjectDiff(oldObject, obj)
		if err != nil {
			diff = fmt.Sprintf("Error generating diff: %v", err)
		}
		logging.FromContext(ctx).Infow("Object last-config-hash has changed",
			zap.String("kind", obj.GetObjectKind().GroupVersionKind().Kind),
			zap.String("obj-name", obj.GetName()),
			zap.String("old-hash", oldObject.GetAnnotations()[util.LastConfigHashAnnotation]),
			zap.String("new-hash", hash),
			zap.String("diff", diff))
	} else {
		logging.FromContext(ctx).Infow(
			"Object meta has changed",
			zap.String("kind", obj.GetObjectKind().GroupVersionKind().Kind),
			zap.String("obj-name", obj.GetName()),
			zap.Any("old-annotations", oldObject.GetAnnotations()),
			zap.Any("new-annotations", obj.GetAnnotations()),
			zap.Any("old-labels", oldObject.GetLabels()),
			zap.Any("new-labels", obj.GetLabels()),
			zap.Any("changed-fileds", objectMetaChangedFields),
		)
	}
}

func EnsureObjectWithHash(
	ctx context.Context,
	cl client.Client,
	owner metav1.Object,
	obj client.Object,
	s *runtime.Scheme,
) error {
	if owner != nil {
		if err := controllerutil.SetControllerReference(owner, obj, s); err != nil {
			return errors.Wrapf(err, "set controller reference to %s/%s",
				obj.GetObjectKind().GroupVersionKind().Kind,
				obj.GetName())
		}
	}

	hash, err := stampLastConfigHash(obj)
	if err != nil {
		return err
	}

	val := reflect.ValueOf(obj)
	if val.Kind() == reflect.Pointer {
		val = reflect.Indirect(val)
	}
	oldObject := reflect.New(val.Type()).Interface().(client.Object)

	nn := types.NamespacedName{
		Name:      obj.GetName(),
		Namespace: obj.GetNamespace(),
	}
	if err = cl.Get(ctx, nn, oldObject); err != nil {
		if !k8serrors.IsNotFound(err) {
			return errors.Wrapf(err, "get %v", nn.String())
		}

		if err := cl.Create(ctx, obj); err != nil {
			return errors.Wrapf(err, "create %v", nn.String())
		}

		return nil
	}

	// Never take over a Service that is not controlled by this owner. Patching it
	// would let us rewrite its selector and redirect live traffic.
	if _, isService := obj.(*corev1.Service); isService && owner != nil {
		if !metav1.IsControlledBy(oldObject, owner) {
			return errors.Wrapf(ErrNotControlledByOwner,
				"service %v is not controlled by %s", nn.String(), owner.GetName())
		}
	}

	if _, isDeployment := obj.(*appsv1.Deployment); isDeployment {
		keepIgnoredAnnotations(obj, oldObject)
	}

	hashChanged := oldObject.GetAnnotations()[util.LastConfigHashAnnotation] != hash
	secretChanged := secretHashChanged(oldObject, obj)
	objectMetaChangedFields := ChangedMetaKeys(obj, oldObject)
	if !hashChanged && len(objectMetaChangedFields) == 0 {
		return nil
	}

	logObjectChange(ctx, oldObject, obj, hash, hashChanged, secretChanged, objectMetaChangedFields)

	obj.SetResourceVersion(oldObject.GetResourceVersion())
	if svc, isService := obj.(*corev1.Service); isService {
		keepServiceImmutableFields(svc, oldObject.(*corev1.Service))
	}

	if job, isJob := obj.(*batchv1.Job); isJob {
		return recreateJob(ctx, cl, job, nn)
	}

	if err := cl.Patch(ctx, obj, patchFor(obj, oldObject)); err != nil {
		return errors.Wrapf(err, "patch %v", nn.String())
	}

	return nil
}

// stampLastConfigHash replaces the last-config-hash annotation of obj with the
// hash of its current content and returns the hash.
func stampLastConfigHash(obj client.Object) (string, error) {
	if obj.GetAnnotations() == nil {
		obj.SetAnnotations(make(map[string]string))
	}

	annotations := obj.GetAnnotations()
	delete(annotations, lastConfigHashAnnotation)
	obj.SetAnnotations(annotations)

	hash, err := ObjectHash(obj)
	if err != nil {
		return "", errors.Wrap(err, "calculate object hash")
	}

	annotations = obj.GetAnnotations()
	annotations[lastConfigHashAnnotation] = hash
	obj.SetAnnotations(annotations)

	return hash, nil
}

// keepIgnoredAnnotations copies annotations that other controllers manage from
// the live object, so they are not seen as a change.
func keepIgnoredAnnotations(obj, old client.Object) {
	annotations := obj.GetAnnotations()
	for _, key := range ignoredAnnotations {
		if v, ok := old.GetAnnotations()[key]; ok {
			annotations[key] = v
		}
	}
	obj.SetAnnotations(annotations)
}

func logObjectChange(
	ctx context.Context,
	obj, old client.Object,
	hash string,
	hashChanged bool,
	changedMetaKeys []string,
) {
	if hashChanged {
		diff, err := ObjectDiff(old, obj)
		if err != nil {
			diff = fmt.Sprintf("Error generating diff: %v", err)
		}
		logging.FromContext(ctx).Infow("Object last-config-hash has changed",
			zap.String("kind", obj.GetObjectKind().GroupVersionKind().Kind),
			zap.String("obj-name", obj.GetName()),
			zap.String("old-hash", old.GetAnnotations()[lastConfigHashAnnotation]),
			zap.String("new-hash", hash),
			zap.String("diff", diff))
		return
	}

	logging.FromContext(ctx).Infow(
		"Object meta has changed",
		zap.String("kind", obj.GetObjectKind().GroupVersionKind().Kind),
		zap.String("obj-name", obj.GetName()),
		zap.Any("old-annotations", old.GetAnnotations()),
		zap.Any("new-annotations", obj.GetAnnotations()),
		zap.Any("old-labels", old.GetLabels()),
		zap.Any("new-labels", obj.GetLabels()),
		zap.Any("changed-fileds", changedMetaKeys),
	)
}

// keepServiceImmutableFields takes fields the API server assigns from the live
// Service.
func keepServiceImmutableFields(svc, old *corev1.Service) {
	svc.Spec.ClusterIP = old.Spec.ClusterIP
	if svc.Spec.Type == corev1.ServiceTypeLoadBalancer {
		svc.Spec.HealthCheckNodePort = old.Spec.HealthCheckNodePort
	}
}

// recreateJob replaces a job. Jobs are special, because they can't be changed
// and should only run once. A job that has succeeded is left alone. Otherwise
// the job is deleted and created again.
func recreateJob(ctx context.Context, cl client.Client, job *batchv1.Job, nn types.NamespacedName) error {
	if job.Status.Succeeded != 0 {
		return nil
	}

	if err := cl.Delete(ctx, job, client.PropagationPolicy("Foreground")); err != nil {
		return errors.Wrapf(err, "delete job %v", nn.String())
	}

	// Wait for deletion. A watch on events would be better, but the manager is not
	// using the correct interface for that.
	for i := 0; i < 10; i++ {
		time.Sleep(time.Second)
		err := cl.Get(ctx, types.NamespacedName{
			Namespace: job.GetNamespace(),
			Name:      job.GetName(),
		}, nil)
		if err != nil {
			if k8serrors.IsNotFound(err) {
				break
			}
			if i == 10 {
				return errors.Wrapf(err, "delete job %v", nn.String())
			}
		}
	}

	job.SetResourceVersion("")
	if err := cl.Create(ctx, job); err != nil {
		return errors.Wrapf(err, "create job %v", nn.String())
	}
	return nil
}

// patchFor builds the patch that turns old into obj.
func patchFor(obj, old client.Object) client.Patch {
	switch oldObj := old.(type) {
	case *cm.Certificate:
		obj.(*cm.Certificate).TypeMeta = oldObj.DeepCopy().TypeMeta
		return client.MergeFrom(oldObj.DeepCopy())
	case *gatewayv1.HTTPRoute:
		// Gateway API resources use MergeFrom instead of StrategicMergeFrom
		obj.(*gatewayv1.HTTPRoute).TypeMeta = oldObj.DeepCopy().TypeMeta
		return client.MergeFrom(oldObj.DeepCopy())
	case *kedav1alpha1.ScaledObject:
		// CRDs don't support strategic merge patch
		obj.(*kedav1alpha1.ScaledObject).TypeMeta = oldObj.DeepCopy().TypeMeta
		return client.MergeFrom(oldObj.DeepCopy())
	case *corev1.Service:
		// The resourceVersion is sent with the patch, so the update fails with a
		// conflict if the Service was replaced or modified since we verified its
		// ownership.
		return client.StrategicMergeFrom(old, client.MergeFromWithOptimisticLock{})
	default:
		return client.StrategicMergeFrom(old)
	}
}

type secretHashData struct {
	Data       map[string][]byte `json:"data,omitempty"`
	StringData map[string]string `json:"stringData,omitempty"`
}

type configMapHashData struct {
	Data       map[string]string `json:"data,omitempty"`
	BinaryData map[string][]byte `json:"binaryData,omitempty"`
}

func extractRelevantData(obj runtime.Object) interface{} {
	switch object := withoutSecretHash(obj).(type) {
	case *appsv1.StatefulSet:
		return object.Spec
	case *appsv1.Deployment:
		return object.Spec
	case *corev1.Service:
		return object.Spec
	case *corev1.Secret:
		return secretHashData{
			Data:       object.Data,
			StringData: object.StringData,
		}
	case *corev1.ConfigMap:
		return configMapHashData{
			Data:       object.Data,
			BinaryData: object.BinaryData,
		}
	case *kedav1alpha1.ScaledObject:
		return object.Spec
	case *cm.Certificate:
		return object.Spec
	case *cm.Issuer:
		return object.Spec
	case *autoscalingv2.HorizontalPodAutoscaler:
		return object.Spec
	case *batchv1.Job:
		return object.Spec
	case *batchv1.CronJob:
		return object.Spec
	case *policy.PodDisruptionBudget:
		return object.Spec
	case *networkingv1.Ingress:
		return object.Spec
	case *gatewayv1.HTTPRoute:
		return object.Spec
	default:
		return obj
	}
}

func ObjectHash(obj runtime.Object) (string, error) {
	dataToMarshal := extractRelevantData(obj)

	data, err := json.Marshal(dataToMarshal)
	if err != nil {
		return "", err
	}

	hash := md5.Sum(data)
	return hex.EncodeToString(hash[:]), nil
}

const (
	redactedValue        = "<redacted>"
	redactedChangedValue = "<redacted, changed>"
)

func ObjectDiff(oldObj, newObj runtime.Object) (string, error) {
	oldData, newData := diffData(oldObj, newObj)

	oldJSON, err := json.Marshal(oldData)
	if err != nil {
		return "", err
	}

	newJSON, err := json.Marshal(newData)
	if err != nil {
		return "", err
	}

	var oldMap, newMap map[string]interface{}
	if err := json.Unmarshal(oldJSON, &oldMap); err != nil {
		return "", err
	}
	if err := json.Unmarshal(newJSON, &newMap); err != nil {
		return "", err
	}

	changes := findChanges(oldMap, newMap, "")
	if len(changes) == 0 {
		return "No changes detected", nil
	}

	var result strings.Builder
	result.WriteString("Changed fields:\n")
	for _, change := range changes {
		result.WriteString(change)
		result.WriteString("\n")
	}

	return result.String(), nil
}

func diffData(oldObj, newObj runtime.Object) (any, any) {
	oldSecret, oldIsSecret := oldObj.(*corev1.Secret)
	newSecret, newIsSecret := newObj.(*corev1.Secret)
	if oldIsSecret && newIsSecret {
		return redactSecrets(oldSecret, newSecret)
	}
	return extractRelevantData(oldObj), extractRelevantData(newObj)
}

func redactSecrets(oldSecret, newSecret *corev1.Secret) (map[string]string, map[string]string) {
	oldValues := secretValues(oldSecret)
	newValues := secretValues(newSecret)

	oldRedacted := make(map[string]string, len(oldValues))
	for key := range oldValues {
		oldRedacted[key] = redactedValue
	}

	newRedacted := make(map[string]string, len(newValues))
	for key, value := range newValues {
		newRedacted[key] = redactedValue
		if oldValue, ok := oldValues[key]; ok && oldValue != value {
			newRedacted[key] = redactedChangedValue
		}
	}
	return oldRedacted, newRedacted
}

func secretValues(s *corev1.Secret) map[string]string {
	values := make(map[string]string)
	if s == nil {
		return values
	}
	for key, value := range s.Data {
		values["data/"+key] = string(value)
	}
	for key, value := range s.StringData {
		values["stringData/"+key] = value
	}
	return values
}

func findChanges(old, new map[string]interface{}, prefix string) []string {
	var changes []string

	for key, newValue := range new {
		fullKey := key
		if prefix != "" {
			fullKey = prefix + "." + key
		}

		oldValue, exists := old[key]
		if !exists {
			changes = append(changes, fmt.Sprintf("  + %s: %v", fullKey, formatValue(newValue)))
			continue
		}

		if !reflect.DeepEqual(oldValue, newValue) {
			oldMap, oldIsMap := oldValue.(map[string]interface{})
			newMap, newIsMap := newValue.(map[string]interface{})

			if oldIsMap && newIsMap {
				changes = append(changes, findChanges(oldMap, newMap, fullKey)...)
			} else {
				changes = append(changes, fmt.Sprintf("  ~ %s: %v -> %v", fullKey, formatValue(oldValue), formatValue(newValue)))
			}
		}
	}

	for key, oldValue := range old {
		fullKey := key
		if prefix != "" {
			fullKey = prefix + "." + key
		}
		if _, exists := new[key]; !exists {
			changes = append(changes, fmt.Sprintf("  - %s: %v", fullKey, formatValue(oldValue)))
		}
	}

	return changes
}

func formatValue(v interface{}) string {
	switch val := v.(type) {
	case string:
		return fmt.Sprintf("%q", val)
	case []interface{}:
		if len(val) > 3 {
			return fmt.Sprintf("[...%d items...]", len(val))
		}
		return fmt.Sprintf("%v", val)
	case map[string]interface{}:
		return "{...}"
	default:
		return fmt.Sprintf("%v", val)
	}
}
