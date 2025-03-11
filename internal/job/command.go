package job

import (
	"context"
	"maps"

	v1 "github.com/shopware/shopware-operator/api/v1"
	"github.com/shopware/shopware-operator/internal/util"
	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

const CONTAINER_NAME_COMMAND = "shopware-command"

func GetCommandJob(
	ctx context.Context,
	client client.Client,
	store v1.Store,
	exec v1.StoreExec,
) (*batchv1.Job, error) {
	mig := CommandJob(store, exec)
	search := &batchv1.Job{
		ObjectMeta: mig.ObjectMeta,
	}
	err := client.Get(ctx, types.NamespacedName{
		Namespace: mig.Namespace,
		Name:      mig.Name,
	}, search)
	return search, err
}

func GetCommandCronJob(
	ctx context.Context,
	client client.Client,
	store v1.Store,
	exec v1.StoreExec,
) (*batchv1.CronJob, error) {
	mig := CommandJob(store, exec)
	search := &batchv1.CronJob{
		ObjectMeta: mig.ObjectMeta,
	}
	err := client.Get(ctx, types.NamespacedName{
		Namespace: mig.Namespace,
		Name:      mig.Name,
	}, search)
	return search, err
}

func CommandCronJob(store v1.Store, ex v1.StoreExec) *batchv1.CronJob {
	// Copy container spec from store to exec
	store.Spec.Container.DeepCopyInto(&ex.Spec.Container)

	labels := util.GetDefaultStoreExecLabels(store, ex)
	labels["shop.shopware.com/storeexec.type"] = "command"
	maps.Copy(labels, util.GetPDBLabels(store))
	annotations := util.GetDefaultContainerExecAnnotations(CONTAINER_NAME_COMMAND, ex)

	job := &batchv1.CronJob{
		TypeMeta: metav1.TypeMeta{
			Kind:       "CronJob",
			APIVersion: "batch/v1",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:        CommandJobName(ex),
			Namespace:   ex.Namespace,
			Labels:      labels,
			Annotations: annotations,
		},
		Spec: batchv1.CronJobSpec{
			Schedule: ex.Spec.CronSchedule,
			Suspend:  &ex.Spec.CronSuspend,
			JobTemplate: batchv1.JobTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{
					Name:        CommandJobName(ex),
					Namespace:   ex.Namespace,
					Labels:      labels,
					Annotations: annotations,
				},
				Spec: getJobSpec(store, ex, labels),
			},
		},
	}

	return job
}

func CommandJob(store v1.Store, ex v1.StoreExec) *batchv1.Job {
	// Copy container spec from store to exec
	store.Spec.Container.DeepCopyInto(&ex.Spec.Container)

	labels := util.GetDefaultStoreExecLabels(store, ex)
	labels["shop.shopware.com/storeexec.type"] = "cron_command"
	annotations := util.GetDefaultContainerExecAnnotations(CONTAINER_NAME_COMMAND, ex)

	job := &batchv1.Job{
		TypeMeta: metav1.TypeMeta{
			Kind:       "Job",
			APIVersion: "batch/v1",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:        CommandJobName(ex),
			Namespace:   ex.Namespace,
			Labels:      labels,
			Annotations: annotations,
		},
		Spec: getJobSpec(store, ex, labels),
	}

	return job
}

func CommandJobName(exec v1.StoreExec) string {
	return exec.Name
}

func getJobSpec(store v1.Store, ex v1.StoreExec, labels map[string]string) batchv1.JobSpec {
	parallelism := int32(1)
	completions := int32(1)
	sharedProcessNamespace := true

	envs := util.MergeEnv(store.GetEnv(), ex.Spec.ExtraEnvs)

	containers := append(store.Spec.Container.ExtraContainers, corev1.Container{
		Name:            CONTAINER_NAME_COMMAND,
		VolumeMounts:    store.Spec.Container.VolumeMounts,
		ImagePullPolicy: store.Spec.Container.ImagePullPolicy,
		Image:           store.Spec.Container.Image,
		Command:         []string{"sh", "-c"},
		Args:            []string{ex.Spec.Script},
		Env:             envs,
	})

	var sa string
	// Global way
	if store.Spec.ServiceAccountName != "" {
		sa = store.Spec.ServiceAccountName
	}
	// Per container way
	if store.Spec.Container.ServiceAccountName != "" {
		sa = store.Spec.Container.ServiceAccountName
	}

	return batchv1.JobSpec{
		Parallelism:  &parallelism,
		Completions:  &completions,
		BackoffLimit: &ex.Spec.MaxRetries,
		Template: corev1.PodTemplateSpec{
			ObjectMeta: metav1.ObjectMeta{
				Labels: labels,
			},
			Spec: corev1.PodSpec{
				ServiceAccountName:            sa,
				ShareProcessNamespace:         &sharedProcessNamespace,
				Volumes:                       store.Spec.Container.Volumes,
				TopologySpreadConstraints:     store.Spec.Container.TopologySpreadConstraints,
				TerminationGracePeriodSeconds: &store.Spec.Container.TerminationGracePeriodSeconds,
				NodeSelector:                  store.Spec.Container.NodeSelector,
				ImagePullSecrets:              store.Spec.Container.ImagePullSecrets,
				RestartPolicy:                 "Never",
				Containers:                    containers,
				SecurityContext:               store.Spec.Container.SecurityContext,
			},
		},
	}
}

// This is just a soft check, use container check for a clean check
// Will return true if container is stopped (Completed, Error)
func IsCommandJobCompleted(
	ctx context.Context,
	c client.Client,
	store v1.Store,
	exec v1.StoreExec,
) (bool, error) {
	job, err := GetCommandJob(ctx, c, store, exec)
	if err != nil {
		if k8serrors.IsNotFound(err) {
			return false, nil
		}
		return false, err
	}

	state, err := IsJobContainerDone(ctx, c, job, CONTAINER_NAME_COMMAND)
	if err != nil {
		return false, err
	}

	return state.IsDone(), nil
}
