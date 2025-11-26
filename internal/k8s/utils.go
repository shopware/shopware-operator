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
	"os"
	"reflect"
	"strings"
	"time"

	cm "github.com/cert-manager/cert-manager/pkg/apis/certmanager/v1"
	"github.com/pkg/errors"
	v1 "github.com/shopware/shopware-operator/api/v1"
	"github.com/shopware/shopware-operator/internal/logging"
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
	gatewayv1 "sigs.k8s.io/gateway-api/apis/v1"
)

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

func EnsureService(
	ctx context.Context,
	cl client.Client,
	owner metav1.Object,
	svc *corev1.Service,
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

func HasObjectChanged(
	ctx context.Context,
	cl client.Client,
	obj client.Object,
) (bool, error) {
	if obj.GetAnnotations() == nil {
		obj.SetAnnotations(make(map[string]string))
	}

	objAnnotations := obj.GetAnnotations()
	delete(objAnnotations, "shopware.com/last-config-hash")
	obj.SetAnnotations(objAnnotations)

	hash, err := ObjectHash(obj)
	if err != nil {
		return true, errors.Wrap(err, "calculate object hash")
	}

	val := reflect.ValueOf(obj)
	if val.Kind() == reflect.Ptr {
		val = reflect.Indirect(val)
	}
	oldObject := reflect.New(val.Type()).Interface().(client.Object)

	nn := types.NamespacedName{
		Name:      obj.GetName(),
		Namespace: obj.GetNamespace(),
	}
	if err = cl.Get(ctx, nn, oldObject); err != nil {
		if !k8serrors.IsNotFound(err) {
			return true, errors.Wrapf(err, "get %v", nn.String())
		}
	}

	oldHash, ok := oldObject.GetAnnotations()["shopware.com/last-config-hash"]
	if !ok || oldHash != hash {
		return true, nil
	}

	ignoreAnnotations := []string{"shopware.com/last-config-hash"}
	switch obj.(type) {
	case *appsv1.Deployment:
		ignoreAnnotations = append(ignoreAnnotations, "deployment.kubernetes.io/revision")
	}

	annotations := obj.GetAnnotations()
	for _, key := range ignoreAnnotations {
		v, ok := oldObject.GetAnnotations()[key]
		if ok {
			annotations[key] = v
		}
	}
	obj.SetAnnotations(annotations)
	return !objectMetaEqual(obj, oldObject), nil
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

	if obj.GetAnnotations() == nil {
		obj.SetAnnotations(make(map[string]string))
	}

	objAnnotations := obj.GetAnnotations()
	delete(objAnnotations, "shopware.com/last-config-hash")
	obj.SetAnnotations(objAnnotations)

	hash, err := ObjectHash(obj)
	if err != nil {
		return errors.Wrap(err, "calculate object hash")
	}

	objAnnotations = obj.GetAnnotations()
	objAnnotations["shopware.com/last-config-hash"] = hash
	obj.SetAnnotations(objAnnotations)

	val := reflect.ValueOf(obj)
	if val.Kind() == reflect.Ptr {
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

	switch obj.(type) {
	case *appsv1.Deployment:
		annotations := obj.GetAnnotations()
		ignoreAnnotations := []string{"deployment.kubernetes.io/revision"}
		for _, key := range ignoreAnnotations {
			v, ok := oldObject.GetAnnotations()[key]
			if ok {
				annotations[key] = v
			}
		}
		obj.SetAnnotations(annotations)
	}

	hashChanged := oldObject.GetAnnotations()["shopware.com/last-config-hash"] != hash
	objectMetaChanged := !objectMetaEqual(obj, oldObject)

	if hashChanged || objectMetaChanged {
		if hashChanged {
			diff, err := ObjectDiff(oldObject, obj)
			if err != nil {
				diff = fmt.Sprintf("Error generating diff: %v", err)
			}
			logging.FromContext(ctx).Infow("Object last-config-hash has changed",
				zap.String("kind", obj.GetObjectKind().GroupVersionKind().Kind),
				zap.String("obj-name", obj.GetName()),
				zap.String("old", oldObject.GetAnnotations()["shopware.com/last-config-hash"]),
				zap.String("new", hash),
				zap.String("diff", diff))
		} else {
			logging.FromContext(ctx).Infow(
				"Object meta has changed",
				zap.String("kind", obj.GetObjectKind().GroupVersionKind().Kind),
				zap.String("obj-name", obj.GetName()),
				zap.Any("oldAnnotations", oldObject.GetAnnotations()),
				zap.Any("newAnnotations", obj.GetAnnotations()),
				zap.Any("oldLabels", oldObject.GetLabels()),
				zap.Any("newLabels", obj.GetLabels()),
			)
		}

		obj.SetResourceVersion(oldObject.GetResourceVersion())
		switch object := obj.(type) {
		case *corev1.Service:
			object.Spec.ClusterIP = oldObject.(*corev1.Service).Spec.ClusterIP
			if object.Spec.Type == corev1.ServiceTypeLoadBalancer {
				object.Spec.HealthCheckNodePort = oldObject.(*corev1.Service).Spec.HealthCheckNodePort
			}
		}

		var patch client.Patch
		switch oldObj := oldObject.(type) {
		case *cm.Certificate:
			patch = client.MergeFrom(oldObj.DeepCopy())
			obj.(*cm.Certificate).TypeMeta = oldObj.DeepCopy().TypeMeta

			// Jobs are more special, because we can't changed them and they should only run once.
			// So if we have no succeeded job we will delete the job and recreate them.
		case *batchv1.Job:
			if obj.(*batchv1.Job).Status.Succeeded != 0 {
				return nil
			}

			if err := cl.Delete(ctx, obj, client.PropagationPolicy("Foreground")); err != nil {
				return errors.Wrapf(err, "delete job %v", nn.String())
			}

			// Wait for deletion. A watch on events would be better, but the manager is not
			// using the correct interface for that.
			for i := 0; i < 10; i++ {
				time.Sleep(time.Second)
				err := cl.Get(ctx, types.NamespacedName{
					Namespace: obj.GetNamespace(),
					Name:      obj.GetName(),
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

			obj.SetResourceVersion("")
			if err := cl.Create(ctx, obj); err != nil {
				return errors.Wrapf(err, "create job %v", nn.String())
			}
			return nil
		case *gatewayv1.HTTPRoute:
			// Gateway API resources use MergeFrom instead of StrategicMergeFrom
			patch = client.MergeFrom(oldObj.DeepCopy())
			obj.(*gatewayv1.HTTPRoute).TypeMeta = oldObj.DeepCopy().TypeMeta
		default:
			patch = client.StrategicMergeFrom(oldObject)
		}

		if err := cl.Patch(ctx, obj, patch); err != nil {
			return errors.Wrapf(err, "patch %v", nn.String())
		}
	}

	return nil
}

func objectMetaEqual(old, new metav1.Object) bool {
	return util.MapEqual(old.GetLabels(), new.GetLabels()) &&
		util.MapEqual(old.GetAnnotations(), new.GetAnnotations())
}

func extractRelevantData(obj runtime.Object) interface{} {
	switch object := obj.(type) {
	case *appsv1.StatefulSet:
		return object.Spec
	case *appsv1.Deployment:
		return object.Spec
	case *corev1.Service:
		return object.Spec
	case *corev1.Secret:
		return object.Data
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

func ObjectDiff(oldObj, newObj runtime.Object) (string, error) {
	oldData := extractRelevantData(oldObj)
	newData := extractRelevantData(newObj)

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
