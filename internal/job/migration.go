package job

import (
	"context"
	"crypto/md5"
	"fmt"

	v1 "github.com/shopware/shopware-operator/api/v1"
	"github.com/shopware/shopware-operator/internal/util"
	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

var MigrationJobIdentifyer = map[string]string{"type": "migration"}

const CONTAINER_NAME_MIGRATION_JOB = "shopware-migration"

func GetMigrationJob(
	ctx context.Context,
	client client.Client,
	store v1.Store,
) (*batchv1.Job, error) {
	mig := MigrationJob(store)
	search := &batchv1.Job{
		ObjectMeta: mig.ObjectMeta,
	}
	err := client.Get(ctx, types.NamespacedName{
		Namespace: mig.Namespace,
		Name:      mig.Name,
	}, search)
	return search, err
}

func MigrationJob(store v1.Store) *batchv1.Job {
	// Merge Overwritten jobContainer fields into container fields
	store.Spec.Container.Merge(store.Spec.MigrationJobContainer)

	backoffLimit := int32(3)
	sharedProcessNamespace := true
	ttlSecondsAfterFinished := int32(86400) // Hardedcoded to 1 day for now

	labels := util.GetDefaultContainerStoreLabels(store, store.Spec.MigrationJobContainer.Labels)
	labels["shop.shopware.com/store.hash"] = GetMigrateHash(store)
	labels["shop.shopware.com/store.type"] = "migration"

	annotations := util.GetDefaultContainerAnnotations(CONTAINER_NAME_MIGRATION_JOB, store, store.Spec.MigrationJobContainer.Annotations)
	annotations["shop.shopware.com/store.oldImage"] = store.Status.CurrentImageTag
	annotations["shop.shopware.com/store.newImage"] = store.Spec.Container.Image

	containers := append(store.Spec.Container.ExtraContainers, corev1.Container{
		Name:            CONTAINER_NAME_MIGRATION_JOB,
		VolumeMounts:    store.Spec.Container.VolumeMounts,
		ImagePullPolicy: store.Spec.Container.ImagePullPolicy,
		Image:           store.Spec.Container.Image,
		Command:         []string{"sh", "-c"},
		Args:            []string{store.Spec.MigrationScript},
		Env:             store.GetEnv(),
	})

	job := &batchv1.Job{
		TypeMeta: metav1.TypeMeta{
			Kind:       "Job",
			APIVersion: "batch/v1",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:        MigrateJobName(store),
			Namespace:   store.Namespace,
			Labels:      labels,
			Annotations: annotations,
		},
		Spec: batchv1.JobSpec{
			BackoffLimit:            &backoffLimit,
			TTLSecondsAfterFinished: &ttlSecondsAfterFinished,
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{
					Labels:      labels,
					Annotations: annotations,
				},
				Spec: corev1.PodSpec{
					ShareProcessNamespace:         &sharedProcessNamespace,
					Volumes:                       store.Spec.Container.Volumes,
					TopologySpreadConstraints:     store.Spec.Container.TopologySpreadConstraints,
					TerminationGracePeriodSeconds: &store.Spec.Container.TerminationGracePeriodSeconds,
					NodeSelector:                  store.Spec.Container.NodeSelector,
					ImagePullSecrets:              store.Spec.Container.ImagePullSecrets,
					RestartPolicy:                 "Never",
					Containers:                    containers,
					SecurityContext:               store.Spec.Container.SecurityContext,
					InitContainers:                store.Spec.Container.InitContainers,
				},
			},
		},
	}

	// Global way
	if store.Spec.ServiceAccountName != "" {
		job.Spec.Template.Spec.ServiceAccountName = store.Spec.ServiceAccountName
	}
	// Per container way
	if store.Spec.Container.ServiceAccountName != "" {
		job.Spec.Template.Spec.ServiceAccountName = store.Spec.Container.ServiceAccountName
	}

	return job
}

func MigrateJobName(store v1.Store) string {
	return fmt.Sprintf("%s-migrate-%s", store.Name, GetMigrateHash(store))
}

func GetMigrateHash(store v1.Store) string {
	data := []byte(store.Status.CurrentImageTag)
	return fmt.Sprintf("%x", md5.Sum(data))
}

func DeleteAllMigrationJobs(ctx context.Context, c client.Client, store *v1.Store) error {
	return deleteJobsByLabel(ctx, c, store.Namespace, MigrationJobIdentifyer)
}

// This is just a soft check, use container check for a clean check
// Will return true if container is stopped (Completed, Error)
func IsMigrationJobCompleted(
	ctx context.Context,
	c client.Client,
	store v1.Store,
) (bool, error) {
	migration, err := GetMigrationJob(ctx, c, store)
	if err != nil {
		if k8serrors.IsNotFound(err) {
			return false, nil
		}
		return false, err
	}

	state, err := IsJobContainerDone(ctx, c, migration, CONTAINER_NAME_MIGRATION_JOB)
	if err != nil {
		return false, err
	}

	return state.IsDone(), nil
}
