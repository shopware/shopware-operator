package deployment

import (
	"context"
	"crypto/sha256"
	"fmt"
	"maps"
	"math"
	"strings"

	v1 "github.com/shopware/shopware-operator/api/v1"
	"github.com/shopware/shopware-operator/internal/util"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/intstr"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

const (
	maxDeploymentNameLength = 63
	maxLabelValueLength     = 63
)

func WorkerQueues(store v1.Store) []string {
	if len(store.Status.QueueState.Transports) == 0 {
		return []string{}
	}

	queues := make([]string, 0, len(store.Status.QueueState.Transports))
	for _, transport := range store.Status.QueueState.Transports {
		queues = append(queues, transport.Name)
	}
	return queues
}

// WorkerDeployments returns one deployment per known queue when perQueue is
// set (keda scaling), otherwise a single deployment consuming all queues.
func WorkerDeployments(store v1.Store, perQueue bool) ([]*appsv1.Deployment, error) {
	queues := WorkerQueues(store)
	for _, queue := range queues {
		if queue == "" {
			return nil, fmt.Errorf("empty queue name for store %s/%s", store.Namespace, store.Name)
		}
	}

	if !perQueue {
		queuesString := strings.Join(queues, " ")
		return []*appsv1.Deployment{WorkerDeployment(store, queuesString)}, nil
	}

	deployments := make([]*appsv1.Deployment, 0, len(queues))
	for _, queue := range queues {
		deployments = append(deployments, WorkerDeployment(store, queue))
	}
	return deployments, nil
}

func CleanupObsoleteWorkerDeployments(
	ctx context.Context,
	c client.Client,
	store v1.Store,
	perQueue bool,
) error {
	workers, err := WorkerDeployments(store, perQueue)
	if err != nil {
		return err
	}
	desired := make(map[string]struct{})
	for _, d := range workers {
		desired[d.Name] = struct{}{}
	}

	list := &appsv1.DeploymentList{}
	if err := c.List(ctx, list,
		client.InNamespace(store.Namespace),
		client.MatchingLabels(util.GetWorkerDeploymentMatchLabel()),
	); err != nil {
		return fmt.Errorf("list worker deployments: %w", err)
	}

	for i := range list.Items {
		d := &list.Items[i]
		if _, ok := desired[d.Name]; ok {
			continue
		}
		if err := c.Delete(ctx, d); err != nil && !k8serrors.IsNotFound(err) {
			return fmt.Errorf("delete obsolete worker deployment %s: %w", d.Name, err)
		}
	}

	return nil
}

func GetWorkerDeploymentCondition(
	ctx context.Context,
	store v1.Store,
	c client.Client,
	perQueue bool,
) v1.DeploymentCondition {
	stateRank := map[v1.DeploymentState]int{
		v1.DeploymentStateRunning:  0,
		v1.DeploymentStateScaling:  1,
		v1.DeploymentStateUnknown:  2,
		v1.DeploymentStateNotFound: 3,
		v1.DeploymentStateError:    4,
	}

	agg := v1.DeploymentCondition{
		State:          v1.DeploymentStateRunning,
		LastUpdateTime: metav1.Now(),
		Message:        "All worker deployments are running",
	}

	workers, err := WorkerDeployments(store, perQueue)
	if err != nil {
		return v1.DeploymentCondition{
			State:          v1.DeploymentStateError,
			LastUpdateTime: metav1.Now(),
			Message:        err.Error(),
			Ready:          "0/0",
		}
	}

	var available, storeReplicas int32
	for _, d := range workers {
		search := &appsv1.Deployment{}
		err := c.Get(ctx, types.NamespacedName{
			Namespace: d.Namespace,
			Name:      d.Name,
		}, search)

		var con v1.DeploymentCondition
		if err != nil {
			if k8serrors.IsNotFound(err) {
				con = v1.DeploymentCondition{
					State:   v1.DeploymentStateNotFound,
					Message: "No deployment found",
				}
			} else {
				con = v1.DeploymentCondition{
					State:   v1.DeploymentStateError,
					Message: fmt.Sprintf("error on client get: %s", err),
				}
			}
		} else {
			con = getDeploymentCondition(search, *d.Spec.Replicas)
			available += search.Status.AvailableReplicas
		}

		storeReplicas += *d.Spec.Replicas
		if stateRank[con.State] > stateRank[agg.State] {
			agg.State = con.State
			agg.Message = fmt.Sprintf("%s: %s", d.Name, con.Message)
		}
	}

	agg.Ready = fmt.Sprintf("%d/%d", available, storeReplicas)
	agg.StoreReplicas = storeReplicas
	return agg
}

func WorkerDeployment(store v1.Store, queue string) *appsv1.Deployment {
	containerSpec := store.Spec.Container.DeepCopy()
	containerSpec.Merge(store.Spec.WorkerDeploymentContainer)
	containerSpec.Volumes = append(containerSpec.Volumes, store.GetDatabaseTLSVolumes()...)
	containerSpec.VolumeMounts = append(containerSpec.VolumeMounts, store.GetDatabaseTLSVolumeMounts()...)

	appName := "shopware-worker"
	matchLabels := util.GetWorkerDeploymentMatchLabel()
	if queue != "" {
		matchLabels[util.ShopwareKey("worker.queue")] = truncateWithHash(queue, maxLabelValueLength)
	}
	labels := util.GetDefaultContainerStoreLabels(store, store.Spec.WorkerDeploymentContainer.Labels)
	maps.Copy(labels, matchLabels)

	annotations := util.GetDefaultContainerAnnotations(appName, store, store.Spec.WorkerDeploymentContainer.Annotations)

	// Worker-specific defaults layered over the shared env, still overridable by
	// ExtraEnvs. The worker runs a few long-lived processes, so persistent DB
	// connections are beneficial here (storefront/admin default to 0 in GetEnv to
	// avoid hoarding connections across many short-lived requests).
	workerDefaults := []corev1.EnvVar{
		{Name: "DATABASE_PERSISTENT_CONNECTION", Value: "1"},
	}
	envs := util.MergeEnv(util.MergeEnv(store.GetEnv(), workerDefaults), containerSpec.ExtraEnvs)

	// Set PHP_MEMORY_LIMIT to 90% of the container memory limit
	phpMemoryLimitMiB := 0
	if containerSpec.Resources.Limits.Memory() != nil && containerSpec.Resources.Limits.Memory().Value() != 0 {
		memoryLimitMiB := containerSpec.Resources.Limits.Memory().Value() / (1024 * 1024)
		phpMemoryLimitMiB = int(math.Floor(float64(memoryLimitMiB) * 0.9))
		envs = util.MergeEnv(envs, []corev1.EnvVar{
			{
				Name:  "PHP_MEMORY_LIMIT",
				Value: fmt.Sprintf("%dM", phpMemoryLimitMiB),
			},
		})
	}

	consume := fmt.Sprintf("bin/console messenger:consume %s --time-limit=300", queue)
	if phpMemoryLimitMiB > 0 {
		consume += fmt.Sprintf(" --memory-limit=%dM", phpMemoryLimitMiB)
	}
	workerScript := fmt.Sprintf(
		`term() {
  trap - TERM INT
  kill -TERM "$child" 2>/dev/null
  wait "$child"
  exit 0
}
trap term TERM INT
while true; do
  %s &
  child=$!
  wait "$child"
done`,
		consume,
	)

	containers := append(util.DefaultContainerSecurityContexts(containerSpec.ExtraContainers), corev1.Container{
		Name:            appName,
		Image:           containerSpec.Image,
		ImagePullPolicy: containerSpec.ImagePullPolicy,
		Env:             envs,
		SecurityContext: util.RestrictedContainerSecurityContext(),
		Command: []string{
			"/bin/sh",
			"-c",
		},
		Args: []string{
			workerScript,
		},
		VolumeMounts: containerSpec.VolumeMounts,
		Ports: []corev1.ContainerPort{
			{
				ContainerPort: containerSpec.Port,
				Protocol:      corev1.ProtocolTCP,
			},
		},
		Resources: containerSpec.Resources,
	})

	deployment := &appsv1.Deployment{
		TypeMeta: metav1.TypeMeta{
			Kind:       "Deployment",
			APIVersion: "apps/v1",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:        GetQueueWorkerDeploymentName(store, queue),
			Namespace:   store.Namespace,
			Labels:      labels,
			Annotations: annotations,
		},
		Spec: appsv1.DeploymentSpec{
			ProgressDeadlineSeconds: &containerSpec.ProgressDeadlineSeconds,
			Replicas:                &containerSpec.Replicas,
			Selector: &metav1.LabelSelector{
				MatchLabels: matchLabels,
			},
			Strategy: appsv1.DeploymentStrategy{
				RollingUpdate: &appsv1.RollingUpdateDeployment{
					MaxSurge: &intstr.IntOrString{
						Type:   intstr.String,
						StrVal: "25%",
					},
					MaxUnavailable: &intstr.IntOrString{
						Type:   intstr.String,
						StrVal: "25%",
					},
				},
			},
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{
					Labels:      labels,
					Annotations: annotations,
				},
				Spec: corev1.PodSpec{
					Volumes:                   containerSpec.Volumes,
					TopologySpreadConstraints: containerSpec.TopologySpreadConstraints,
					NodeSelector:              containerSpec.NodeSelector,
					ImagePullSecrets:          containerSpec.ImagePullSecrets,
					EnableServiceLinks:        containerSpec.EnableServiceLinks,
					RestartPolicy:             containerSpec.RestartPolicy,
					Containers:                containers,
					SecurityContext:           util.DefaultPodSecurityContext(containerSpec.SecurityContext),
					InitContainers:            util.DefaultContainerSecurityContexts(containerSpec.InitContainers),
				},
			},
		},
	}

	// Old way
	if store.Spec.ServiceAccountName != "" {
		deployment.Spec.Template.Spec.ServiceAccountName = store.Spec.ServiceAccountName
	}
	// New way
	if containerSpec.ServiceAccountName != "" {
		deployment.Spec.Template.Spec.ServiceAccountName = containerSpec.ServiceAccountName
	}

	return deployment
}

func GetWorkerDeploymentName(store v1.Store) string {
	return fmt.Sprintf("%s-store-worker", store.Name)
}

func GetQueueWorkerDeploymentName(store v1.Store, queue string) string {
	if queue == "" {
		return GetWorkerDeploymentName(store)
	}
	sanitized := strings.ReplaceAll(strings.ToLower(queue), "_", "-")
	name := fmt.Sprintf("%s-%s", GetWorkerDeploymentName(store), sanitized)
	return truncateWithHash(name, maxDeploymentNameLength)
}

// truncateWithHash keeps names within the k8s limit while staying unique and
// deterministic: the overlong name is cut and suffixed with a hash of itself.
func truncateWithHash(name string, maxLength int) string {
	if len(name) <= maxLength {
		return name
	}
	hash := fmt.Sprintf("%x", sha256.Sum256([]byte(name)))[:8]
	return strings.TrimRight(name[:maxLength-9], "-") + "-" + hash
}
