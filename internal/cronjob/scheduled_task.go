package cronjob

import (
	"context"
	"fmt"

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
	store.Spec.Container.Merge(store.Spec.SetupJobContainer)

	parallelism := int32(1)
	completions := int32(1)
	sharedProcessNamespace := true
	var sa string

	// Global way
	if store.Spec.ServiceAccountName != "" {
		sa = store.Spec.ServiceAccountName
	}
	// Per container way
	if store.Spec.Container.ServiceAccountName != "" {
		sa = store.Spec.Container.ServiceAccountName
	}

	labels := util.GetDefaultContainerStoreLabels(store, store.Spec.SetupJobContainer.Labels)
	labels["shop.shopware.com/store.type"] = "scheduled-task"

	annotations := util.GetDefaultContainerAnnotations(CONTAINER_NAME_SCHEDULED_JOB, store, store.Spec.SetupJobContainer.Annotations)

	containers := append(store.Spec.Container.ExtraContainers, corev1.Container{
		Name:            CONTAINER_NAME_SCHEDULED_JOB,
		VolumeMounts:    store.Spec.Container.VolumeMounts,
		ImagePullPolicy: store.Spec.Container.ImagePullPolicy,
		Image:           store.Spec.Container.Image,
		Command:         []string{"sh", "-c"},
		Args:            []string{store.Spec.ScheduledTask.Command},
		Env:             store.GetEnv(),
		Resources:       store.Spec.Container.Resources, // Add Resources here
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
							TerminationGracePeriodSeconds: &store.Spec.Container.TerminationGracePeriodSeconds,
							Volumes:                       store.Spec.Container.Volumes,
							TopologySpreadConstraints:     store.Spec.Container.TopologySpreadConstraints,
							NodeSelector:                  store.Spec.Container.NodeSelector,
							ImagePullSecrets:              store.Spec.Container.ImagePullSecrets,
							RestartPolicy:                 "Never",
							Containers:                    containers,
							SecurityContext:               store.Spec.Container.SecurityContext,
							ServiceAccountName:            sa,
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
