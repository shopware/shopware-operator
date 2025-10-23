package job

import (
	"context"
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

const CONTAINER_NAME_SETUP_JOB = "shopware-setup"

func GetSetupJob(ctx context.Context, client client.Client, store v1.Store) (*batchv1.Job, error) {
	setup := SetupJob(store)
	search := &batchv1.Job{
		ObjectMeta: setup.ObjectMeta,
	}
	err := client.Get(ctx, types.NamespacedName{
		Namespace: setup.Namespace,
		Name:      setup.Name,
	}, search)
	return search, err
}

func SetupJob(store v1.Store) *batchv1.Job {
	containerSpec := store.Spec.Container.DeepCopy()
	containerSpec.Merge(store.Spec.SetupJobContainer)

	sharedProcessNamespace := true
	backoffLimit := int32(3)

	labels := util.GetDefaultContainerStoreLabels(store, store.Spec.MigrationJobContainer.Labels)
	labels["shop.shopware.com/store.type"] = "setup"

	// Use util function for annotations
	annotations := util.GetDefaultContainerAnnotations(CONTAINER_NAME_SETUP_JOB, store, store.Spec.SetupJobContainer.Annotations)

	envs := append(store.GetEnv(),
		corev1.EnvVar{
			Name: "INSTALL_ADMIN_PASSWORD",
			ValueFrom: &corev1.EnvVarSource{
				SecretKeyRef: &corev1.SecretKeySelector{
					LocalObjectReference: corev1.LocalObjectReference{
						Name: store.GetSecretName(),
					},
					Key: "admin-password",
				},
			},
		},
		corev1.EnvVar{
			Name:  "INSTALL_ADMIN_USERNAME",
			Value: store.Spec.AdminCredentials.Username,
		},
	)

	// Merge containerSpec.ExtraEnvs to override with merged values from SetupJobContainer
	envs = util.MergeEnv(envs, containerSpec.ExtraEnvs)

	containers := append(containerSpec.ExtraContainers, corev1.Container{
		Name:            CONTAINER_NAME_SETUP_JOB,
		VolumeMounts:    containerSpec.VolumeMounts,
		ImagePullPolicy: containerSpec.ImagePullPolicy,
		Image:           containerSpec.Image,
		Command:         []string{"sh", "-c"},
		Args:            []string{store.Spec.SetupScript},
		Env:             envs,
		Resources:       containerSpec.Resources, // Add Resources here
	})

	job := &batchv1.Job{
		TypeMeta: metav1.TypeMeta{
			Kind:       "Job",
			APIVersion: "batch/v1",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:        GetSetupJobName(store),
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
					TerminationGracePeriodSeconds: &containerSpec.TerminationGracePeriodSeconds,
					Volumes:                       containerSpec.Volumes,
					TopologySpreadConstraints:     containerSpec.TopologySpreadConstraints,
					NodeSelector:                  containerSpec.NodeSelector,
					ImagePullSecrets:              containerSpec.ImagePullSecrets,
					RestartPolicy:                 "Never",
					Containers:                    containers,
					SecurityContext:               containerSpec.SecurityContext,
					InitContainers:                containerSpec.InitContainers,
				},
			},
		},
	}

	// Global way
	if store.Spec.ServiceAccountName != "" {
		job.Spec.Template.Spec.ServiceAccountName = store.Spec.ServiceAccountName
	}
	// Per container way
	if containerSpec.ServiceAccountName != "" {
		job.Spec.Template.Spec.ServiceAccountName = containerSpec.ServiceAccountName
	}

	return job
}

func GetSetupJobName(store v1.Store) string {
	return fmt.Sprintf("%s-setup", store.Name)
}

func DeleteSetupJob(ctx context.Context, c client.Client, store v1.Store) error {
	job, err := GetSetupJob(ctx, c, store)
	if err != nil {
		if k8serrors.IsNotFound(err) {
			return nil
		}
		return err
	}

	return c.Delete(ctx, job, client.PropagationPolicy("Foreground"))
}

// This is just a soft check, use container check for a clean check
// Will return true if container is stopped (Completed, Error)
func IsSetupJobCompleted(
	ctx context.Context,
	c client.Client,
	store v1.Store,
) (bool, error) {
	setup, err := GetSetupJob(ctx, c, store)
	if err != nil {
		if k8serrors.IsNotFound(err) {
			return false, nil
		}
		return false, err
	}

	state, err := IsJobContainerDone(ctx, c, setup, CONTAINER_NAME_SETUP_JOB)
	if err != nil {
		return false, err
	}

	return state.IsDone(), nil
}
