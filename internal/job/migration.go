package job

import (
	"context"
	"crypto/md5"
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

func GetMigrationJob(ctx context.Context, store *v1.Store, client client.Client) (*batchv1.Job, error) {
	mig := MigrationJob(store)
	search := &batchv1.Job{
		ObjectMeta: mig.ObjectMeta,
	}
	err := client.Get(ctx, types.NamespacedName{
		Namespace: mig.Namespace,
	}, search)
	if err != nil && errors.IsNotFound(err) {
		return nil, nil
	}
	return search, err
}

func MigrationJob(store *v1.Store) *batchv1.Job {
	parallelism := int32(1)
	completions := int32(1)

	labels := map[string]string{
		"type": "migration",
	}
	maps.Copy(labels, util.GetDefaultLabels(store))

	annotations := map[string]string{
		"oldImage": store.Status.CurrentImageTag,
		"newImage": store.Spec.Container.Image,
	}

	var command string
	if store.Spec.MigrationHook.Before != "" {
		command = fmt.Sprintf("%s && ", store.Spec.MigrationHook.Before)
	}
	command = fmt.Sprintf("%s /setup", command)
	if store.Spec.MigrationHook.After != "" {
		command = fmt.Sprintf("%s && %s", command, store.Spec.MigrationHook.After)
	}

	return &batchv1.Job{
		TypeMeta: metav1.TypeMeta{
			Kind:       "Job",
			APIVersion: "batch/v1"},
		ObjectMeta: metav1.ObjectMeta{
			Name:        MigrateJobName(store),
			Namespace:   store.Namespace,
			Labels:      labels,
			Annotations: annotations,
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
							Name:            "shopware-migration",
							ImagePullPolicy: store.Spec.Container.ImagePullPolicy,
							Image:           store.Spec.Container.Image,
							Command:         []string{"sh", "-c"},
							Args:            []string{command},
							Env:             store.GetEnv(),
						},
					},
				},
			},
		},
	}
}

func MigrateJobName(store *v1.Store) string {
	data := []byte(fmt.Sprintf("%s|%s", store.Status.CurrentImageTag, store.Spec.Container.Image))
	return fmt.Sprintf("%s-migrate-%x", store.Name, md5.Sum(data))
}
