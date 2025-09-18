package job

import (
	"context"
	"fmt"

	v1 "github.com/shopware/shopware-operator/api/v1"
	"github.com/shopware/shopware-operator/internal/util"
	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

const CONTAINER_NAME_SNAPSHOT = "operator-snapshot"

func GetSnapshotCreateJob(
	ctx context.Context,
	client client.Client,
	store v1.Store,
	snap v1.StoreSnapshotCreate,
) (*batchv1.Job, error) {
	mig := SnapshotCreateJob(store, snap)
	search := &batchv1.Job{
		ObjectMeta: mig.ObjectMeta,
	}
	err := client.Get(ctx, types.NamespacedName{
		Namespace: mig.Namespace,
		Name:      mig.Name,
	}, search)
	return search, err
}

func SnapshotCreateJob(store v1.Store, snapshot v1.StoreSnapshotCreate) *batchv1.Job {
	return snapshotJob(store, snapshot.ObjectMeta, snapshot.Spec, "create")
}

func GetSnapshotRestoreJob(
	ctx context.Context,
	client client.Client,
	store v1.Store,
	snap v1.StoreSnapshotRestore,
) (*batchv1.Job, error) {
	mig := SnapshotRestoreJob(store, snap)
	search := &batchv1.Job{
		ObjectMeta: mig.ObjectMeta,
	}
	err := client.Get(ctx, types.NamespacedName{
		Namespace: mig.Namespace,
		Name:      mig.Name,
	}, search)
	return search, err
}

func SnapshotRestoreJob(store v1.Store, snapshot v1.StoreSnapshotRestore) *batchv1.Job {
	return snapshotJob(store, snapshot.ObjectMeta, snapshot.Spec, "restore")
}

func snapshotJob(store v1.Store, meta metav1.ObjectMeta, snapshot v1.StoreSnapshotSpec, subCommand string) *batchv1.Job {
	sharedProcessNamespace := true
	res := resource.MustParse("20Gi")

	vm := append(snapshot.Container.VolumeMounts, corev1.VolumeMount{
		Name:      "tempdir",
		ReadOnly:  false,
		MountPath: "/temp",
	})
	containers := append(snapshot.Container.ExtraContainers, corev1.Container{
		Name:            CONTAINER_NAME_SNAPSHOT,
		VolumeMounts:    vm,
		ImagePullPolicy: snapshot.Container.ImagePullPolicy,
		Image:           snapshot.Container.Image,
		Args: []string{
			subCommand,
			"--backup-file", snapshot.Path,
			"--tempdir", "/temp",
		},
		Env: snapshot.GetEnv(store),
	})

	res := resource.MustParse("20Gi")
	volumes := append(snapshot.Container.Volumes, corev1.Volume{
		Name: "tempdir",
		VolumeSource: corev1.VolumeSource{
			EmptyDir: &corev1.EmptyDirVolumeSource{
				SizeLimit: &res,
			},
		},
	})

	labels := util.GetDefaultStoreSnapshotLabels(store, meta)
	labels["shop.shopware.com/storesnapshot.type"] = "create"
	annotations := util.GetDefaultContainerSnapshotAnnotations(CONTAINER_NAME_SNAPSHOT, snapshot)

	job := &batchv1.Job{
		TypeMeta: metav1.TypeMeta{
			Kind:       "Job",
			APIVersion: "batch/v1",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:        SnapshotJobName(store, meta, subCommand),
			Namespace:   meta.Namespace,
			Labels:      labels,
			Annotations: annotations,
		},
		Spec: batchv1.JobSpec{
			BackoffLimit: &snapshot.MaxRetries,
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{
					Labels: labels,
				},
				Spec: corev1.PodSpec{
					ServiceAccountName:            snapshot.Container.ServiceAccountName,
					ShareProcessNamespace:         &sharedProcessNamespace,
					Volumes:                       volumes,
					TopologySpreadConstraints:     snapshot.Container.TopologySpreadConstraints,
					TerminationGracePeriodSeconds: &snapshot.Container.TerminationGracePeriodSeconds,
					NodeSelector:                  snapshot.Container.NodeSelector,
					ImagePullSecrets:              snapshot.Container.ImagePullSecrets,
					RestartPolicy:                 "Never",
					Containers:                    containers,
					SecurityContext:               snapshot.Container.SecurityContext,
					InitContainers:                snapshot.Container.InitContainers,
				},
			},
		},
	}

	return job
}

func SnapshotJobName(store v1.Store, snap metav1.ObjectMeta, subCommand string) string {
	return fmt.Sprintf("%s-snapshot-%s-%s", store.Name, subCommand, snap.Name)
}

func IsSnapshotJobCompleted(
	ctx context.Context,
	c client.Client,
	j *batchv1.Job,
) (bool, error) {
	state, err := IsJobContainerDone(ctx, c, j, CONTAINER_NAME_SNAPSHOT)
	if err != nil {
		return false, err
	}

	return state.IsDone(), nil
}
