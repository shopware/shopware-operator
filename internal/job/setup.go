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

const CONTAINER_NAME_SETUP_JOB = "shopware-setup"

func GetSetupJob(ctx context.Context, client client.Client, store *v1.Store) (*batchv1.Job, error) {
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

func SetupJob(store *v1.Store) *batchv1.Job {
	parallelism := int32(1)
	completions := int32(1)
	sharedProcessNamespace := true

	labels := map[string]string{
		"type": "setup",
	}
	maps.Copy(labels, util.GetDefaultLabels(store))

	var command []string
	if store.Spec.SetupHook.Before != "" {
		command = append(command, store.Spec.SetupHook.Before)
	}
	command = append(command, "/setup")
	if store.Spec.SetupHook.After != "" {
		command = append(command, store.Spec.SetupHook.After)
	}

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

	containers := append(store.Spec.Container.ExtraContainers, corev1.Container{
		Name:            CONTAINER_NAME_SETUP_JOB,
		VolumeMounts:    store.Spec.Container.VolumeMounts,
		ImagePullPolicy: store.Spec.Container.ImagePullPolicy,
		Image:           store.Spec.Container.Image,
		Command:         []string{"sh", "-c"},
		Args:            command,
		Env:             envs,
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
			Annotations: store.Spec.Container.Annotations,
		},
		Spec: batchv1.JobSpec{
			Parallelism: &parallelism,
			Completions: &completions,
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{
					Labels: labels,
				},
				Spec: corev1.PodSpec{
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

	if store.Spec.ServiceAccountName != "" {
		job.Spec.Template.Spec.ServiceAccountName = store.Spec.ServiceAccountName
	}

	return job
}

func GetSetupJobName(store *v1.Store) string {
	return fmt.Sprintf("%s-setup", store.Name)
}

func DeleteSetupJob(ctx context.Context, c client.Client, store *v1.Store) error {
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
	store *v1.Store,
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
