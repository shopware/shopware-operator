package job

import (
	"context"
	"fmt"

	v1 "github.com/shopware/shopware-operator/api/v1"
	"github.com/shopware/shopware-operator/internal/util"
	"golang.org/x/exp/maps"
	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

func GetSetupJob(ctx context.Context, store *v1.Store, client client.Client) (*batchv1.Job, error) {
	setup := SetupJob(store)
	search := &batchv1.Job{
		ObjectMeta: setup.ObjectMeta,
	}
	err := client.Get(ctx, types.NamespacedName{
		Namespace: setup.Namespace,
		Name:      setup.Name,
	}, search)
	if err != nil && errors.IsNotFound(err) {
		return nil, nil
	}
	return search, err
}

func SetupJob(store *v1.Store) *batchv1.Job {
	parallelism := int32(1)
	completions := int32(1)

	labels := map[string]string{
		"type": "setup",
	}
	maps.Copy(labels, util.GetDefaultLabels(store))

	var command string
	if store.Spec.SetupHook.Before != "" {
		command = fmt.Sprintf("%s && ", store.Spec.SetupHook.Before)
	}
	command = fmt.Sprintf("%s /setup", command)
	if store.Spec.SetupHook.After != "" {
		command = fmt.Sprintf("%s && %s", command, store.Spec.SetupHook.After)
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

	return &batchv1.Job{
		TypeMeta: metav1.TypeMeta{
			Kind:       "Job",
			APIVersion: "batch/v1",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      SetupJobName(store),
			Namespace: store.Namespace,
			Labels:    labels,
		},
		Spec: batchv1.JobSpec{
			Parallelism: &parallelism,
			Completions: &completions,
			Template: corev1.PodTemplateSpec{
				Spec: corev1.PodSpec{
					NodeSelector:     store.Spec.Container.NodeSelector,
					ImagePullSecrets: store.Spec.Container.ImagePullSecrets,
					RestartPolicy:    "Never",
					Containers: []corev1.Container{
						{
							Name:            "shopware-setup",
							ImagePullPolicy: store.Spec.Container.ImagePullPolicy,
							Image:           store.Spec.Container.Image,
							Command:         []string{"sh", "-c"},
							Args:            []string{command},
							Env:             envs,
						},
					},
				},
			},
		},
	}
}

func SetupJobName(store *v1.Store) string {
	return fmt.Sprintf("%s-setup", store.Name)
}
