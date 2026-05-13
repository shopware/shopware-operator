package cronjob

import (
	"context"
	"fmt"
	"maps"

	v1 "github.com/shopware/shopware-operator/api/v1"
	"github.com/shopware/shopware-operator/internal/util"
	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

const CONTAINER_NAME_SCHEDULED_JOB = "shopware-scheduled-task"

func GetScheduledCronJob(ctx context.Context, client client.Client, store v1.Store) (*batchv1.CronJob, error) {
	setup := ScheduledTaskJob(store)
	search := &batchv1.CronJob{
		ObjectMeta: setup.ObjectMeta,
	}
	err := client.Get(ctx, types.NamespacedName{
		Namespace: setup.Namespace,
		Name:      setup.Name,
	}, search)
	return search, err
}

func ScheduledTaskJob(store v1.Store) *batchv1.CronJob {
	// Merge Overwritten jobContainer fields into container fields
	containerSpec := store.Spec.Container.DeepCopy()
	containerSpec.Merge(store.Spec.SetupJobContainer)

	parallelism := int32(1)
	completions := int32(1)
	sharedProcessNamespace := true
	var sa string

	// Global way
	if store.Spec.ServiceAccountName != "" {
		sa = store.Spec.ServiceAccountName
	}
	// Per container way
	if containerSpec.ServiceAccountName != "" {
		sa = containerSpec.ServiceAccountName
	}

	labels := util.GetDefaultContainerStoreLabels(store, nil)
	labels["component"] = "scheduled-task"
	labels[util.ShopwareKey("store.type")] = "scheduled-task"

	// Hack: The current size of the CRD is at the limit.
	// The clean way would be to have ScheduledTaskContainer ContainerMergeSpec `json:"scheduledTaskContainer,omitempty"`
	// in the CRD and merge it like the SetupJobContainer but then the CRD is too big for ETCD
	// This will be removed once the CRD definition is refactored to consume less space
	if store.Spec.ScheduledTaskLabels != nil {
		maps.Copy(labels, store.Spec.ScheduledTaskLabels)
	}

	annotations := util.GetDefaultContainerAnnotations(CONTAINER_NAME_SCHEDULED_JOB, store, store.Spec.SetupJobContainer.Annotations)
	envs := util.MergeEnv(store.GetEnv(), containerSpec.ExtraEnvs)

	containers := append(containerSpec.ExtraContainers, corev1.Container{
		Name:            CONTAINER_NAME_SCHEDULED_JOB,
		VolumeMounts:    containerSpec.VolumeMounts,
		ImagePullPolicy: containerSpec.ImagePullPolicy,
		Image:           containerSpec.Image,
		Command:         []string{"sh", "-c"},
		Args:            []string{store.Spec.ScheduledTask.Command},
		Env:             envs,
		Resources:       containerSpec.Resources, // Add Resources here
	})

	job := &batchv1.CronJob{
		TypeMeta: metav1.TypeMeta{
			Kind:       "CronJob",
			APIVersion: "batch/v1",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:        GetScheduledCronJobName(store),
			Namespace:   store.Namespace,
			Labels:      labels,
			Annotations: annotations,
		},
		Spec: batchv1.CronJobSpec{
			Schedule:          store.Spec.ScheduledTask.Schedule,
			TimeZone:          &store.Spec.ScheduledTask.TimeZone,
			ConcurrencyPolicy: "Forbid",
			Suspend:           &store.Spec.ScheduledTask.Suspend,
			JobTemplate: batchv1.JobTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{
					Name:        GetScheduledCronJobName(store),
					Namespace:   store.Namespace,
					Labels:      labels,
					Annotations: annotations,
				},
				Spec: batchv1.JobSpec{
					Parallelism: &parallelism,
					Completions: &completions,
					Template: corev1.PodTemplateSpec{
						ObjectMeta: metav1.ObjectMeta{
							Labels:      labels,
							Annotations: annotations,
						},
						Spec: corev1.PodSpec{
							ShareProcessNamespace:         &sharedProcessNamespace,
							TerminationGracePeriodSeconds: &containerSpec.TerminationGracePeriodSeconds,
							Volumes:                       containerSpec.Volumes,
							TopologySpreadConstraints:     containerSpec.TopologySpreadConstraints,
							NodeSelector:                  containerSpec.NodeSelector,
							ImagePullSecrets:              containerSpec.ImagePullSecrets,
							EnableServiceLinks:            containerSpec.EnableServiceLinks,
							RestartPolicy:                 "Never",
							Containers:                    containers,
							SecurityContext:               containerSpec.SecurityContext,
							ServiceAccountName:            sa,
							InitContainers:                containerSpec.InitContainers,
						},
					},
				},
			},
		},
	}

	return job
}

func GetScheduledCronJobName(store v1.Store) string {
	return fmt.Sprintf("%s-scheduled-jobs", store.Name)
}
