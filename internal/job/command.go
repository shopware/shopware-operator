package job

import (
	"context"
	"fmt"

	v1 "github.com/shopware/shopware-operator/api/v1"
	"github.com/shopware/shopware-operator/internal/util"
	"golang.org/x/exp/maps"
	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

var CommandJobIdendtifier = map[string]string{"type": "command"}

const CONTAINER_NAME_COMMAND = "shopware-command"

func GetCommandJob(
	ctx context.Context,
	client client.Client,
	store *v1.Store,
	exec *v1.StoreExec,
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

func CommandJob(store *v1.Store, ex *v1.StoreExec) *batchv1.Job {
	parallelism := int32(1)
	completions := int32(1)
	sharedProcessNamespace := true

	labels := util.GetDefaultStoreExecLabels(ex)
	maps.Copy(labels, CommandJobIdendtifier)

	annotations := map[string]string{}
	maps.Copy(annotations, ex.Spec.Container.Annotations)

	envs := util.MergeEnv(store.GetEnv(), ex.Spec.ExtraEnvs)

	// Copy container spec from store to exec
	store.Spec.Container.DeepCopyInto(&ex.Spec.Container)

	containers := append(store.Spec.Container.ExtraContainers, corev1.Container{
		Name:            CONTAINER_NAME_COMMAND,
		VolumeMounts:    store.Spec.Container.VolumeMounts,
		ImagePullPolicy: store.Spec.Container.ImagePullPolicy,
		Image:           store.Spec.Container.Image,
		Command:         []string{"sh", "-c"},
		Args:            []string{ex.Spec.Script},
		Env:             envs,
	})

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
		Spec: batchv1.JobSpec{
			Parallelism: &parallelism,
			Completions: &completions,
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{
					Labels: labels,
				},
				Spec: corev1.PodSpec{
					ServiceAccountName:        store.Spec.Container.ServiceAccountName,
					ShareProcessNamespace:     &sharedProcessNamespace,
					Volumes:                   store.Spec.Container.Volumes,
					TopologySpreadConstraints: store.Spec.Container.TopologySpreadConstraints,
					NodeSelector:              store.Spec.Container.NodeSelector,
					ImagePullSecrets:          store.Spec.Container.ImagePullSecrets,
					RestartPolicy:             "Never",
					Containers:                containers,
					SecurityContext:           store.Spec.Container.SecurityContext,
				},
			},
		},
	}

	j := &batchv1.CronJob{Spec: batchv1.CronJobSpec{
		Schedule:                "",
		TimeZone:                new(string),
		StartingDeadlineSeconds: new(int64),
		ConcurrencyPolicy:       "",
		Suspend:                 new(bool),
		JobTemplate: batchv1.JobTemplateSpec{
			ObjectMeta: job.ObjectMeta,
			Spec:       job.Spec,
		},
		SuccessfulJobsHistoryLimit: new(int32),
		FailedJobsHistoryLimit:     new(int32),
	}}
	fmt.Println(j)

	return job
}

func CommandJobName(exec *v1.StoreExec) string {
	return fmt.Sprintf("%s", exec.Name)
}

// This is just a soft check, use container check for a clean check
// Will return true if container is stopped (Completed, Error)
func IsCommandJobCompleted(
	ctx context.Context,
	c client.Client,
	store *v1.Store,
	exec *v1.StoreExec,
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
