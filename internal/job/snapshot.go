package job

import (
	"context"
	"fmt"
	"sort"

	v1 "github.com/shopware/shopware-operator/api/v1"
	snapshottool "github.com/shopware/shopware-operator/internal/snapshot"
	"github.com/shopware/shopware-operator/internal/util"
	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

const (
	CONTAINER_NAME_SNAPSHOT = "operator-snapshot"
	DEFAULT_TEMP_SIZE       = "20Gi"
	TTLSecondsAfterFinished = 18 * 60 * 60 // 15 minutes
)

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
	res := resource.MustParse(DEFAULT_TEMP_SIZE)
	tempDirMountPath := "/temp"
	addDefaultTempDir := true

	for _, volume := range snapshot.Container.Volumes {
		if volume.Ephemeral == nil {
			continue
		}

		for _, mount := range snapshot.Container.VolumeMounts {
			if mount.Name == volume.Name && mount.MountPath != "" {
				tempDirMountPath = mount.MountPath
				addDefaultTempDir = false
				break
			}
		}

		if !addDefaultTempDir {
			break
		}
	}

	vm := snapshot.Container.VolumeMounts
	vm = append(vm, store.GetDatabaseTLSVolumeMounts()...)
	if addDefaultTempDir {
		vm = append(vm, corev1.VolumeMount{
			Name:      "tempdir",
			ReadOnly:  false,
			MountPath: tempDirMountPath,
		})
	}
	args := []string{
		subCommand,
		"--backup-file", snapshot.Path,
		"--tempdir", tempDirMountPath,
	}
	if addDefaultTempDir {
		args = append(args, "--temp-size", res.String())
	} else if qty, ok := ephemeralVolumeStorageRequest(snapshot); ok {
		args = append(args, "--temp-size", qty.String())
	}
	// Add labels from meta
	// We sort the labels to ensure consistent ordering, so we don't trigger unnecessary job restarts due to label order changes
	if meta.Labels != nil {
		labelKeys := make([]string, 0, len(meta.Labels))
		for key := range meta.Labels {
			labelKeys = append(labelKeys, key)
		}
		sort.Strings(labelKeys)

		for _, key := range labelKeys {
			if util.NormalizeSnapshotLabelKey(key) == "namespace" {
				continue
			}
			value := meta.Labels[key]
			args = append(args, "--label", fmt.Sprintf("%s=%s", util.NormalizeSnapshotLabelKey(key), value))
		}
	}
	if meta.Namespace != "" {
		args = append(args, "--label", fmt.Sprintf("namespace=%s", meta.Namespace))
	}

	containers := append(util.DefaultContainerSecurityContexts(snapshot.Container.ExtraContainers), corev1.Container{
		Name:            CONTAINER_NAME_SNAPSHOT,
		VolumeMounts:    vm,
		ImagePullPolicy: snapshot.Container.ImagePullPolicy,
		Image:           snapshot.Container.Image,
		SecurityContext: util.RestrictedContainerSecurityContext(),
		Args:            args,
		Env:             snapshot.GetEnv(store),
		Resources:       snapshot.Container.Resources,
	})

	volumes := snapshot.Container.Volumes
	volumes = append(volumes, store.GetDatabaseTLSVolumes()...)
	if addDefaultTempDir {
		volumes = append(volumes, corev1.Volume{
			Name: "tempdir",
			VolumeSource: corev1.VolumeSource{
				EmptyDir: &corev1.EmptyDirVolumeSource{
					SizeLimit: &res,
				},
			},
		})
	}

	labels := util.GetDefaultStoreSnapshotLabels(store, meta.Labels, meta.Name, subCommand)
	annotations := util.GetDefaultContainerSnapshotAnnotations(CONTAINER_NAME_SNAPSHOT, snapshot)

	job := &batchv1.Job{
		TypeMeta: metav1.TypeMeta{
			Kind:       "Job",
			APIVersion: "batch/v1",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:        SnapshotJobName(meta, subCommand),
			Namespace:   meta.Namespace,
			Labels:      labels,
			Annotations: annotations,
		},
		Spec: batchv1.JobSpec{
			BackoffLimit:            &snapshot.MaxRetries,
			TTLSecondsAfterFinished: func(i int32) *int32 { return &i }(TTLSecondsAfterFinished),
			PodFailurePolicy:        snapshotNoSpaceFailurePolicy(),
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{
					Labels:      labels,
					Annotations: annotations,
				},
				Spec: corev1.PodSpec{
					ServiceAccountName:            snapshot.Container.ServiceAccountName,
					ShareProcessNamespace:         &sharedProcessNamespace,
					Volumes:                       volumes,
					TopologySpreadConstraints:     snapshot.Container.TopologySpreadConstraints,
					TerminationGracePeriodSeconds: &snapshot.Container.TerminationGracePeriodSeconds,
					NodeSelector:                  snapshot.Container.NodeSelector,
					ImagePullSecrets:              snapshot.Container.ImagePullSecrets,
					EnableServiceLinks:            snapshot.Container.EnableServiceLinks,
					RestartPolicy:                 "Never",
					Containers:                    containers,
					SecurityContext:               util.DefaultPodSecurityContext(snapshot.Container.SecurityContext),
					InitContainers:                util.DefaultContainerSecurityContexts(snapshot.Container.InitContainers),
				},
			},
		},
	}

	return job
}

func ephemeralVolumeStorageRequest(spec v1.StoreSnapshotSpec) (resource.Quantity, bool) {
	for _, volume := range spec.Container.Volumes {
		if volume.Ephemeral == nil || volume.Ephemeral.VolumeClaimTemplate == nil {
			continue
		}
		qty, ok := volume.Ephemeral.VolumeClaimTemplate.Spec.Resources.Requests[corev1.ResourceStorage]
		if ok && !qty.IsZero() {
			return qty, true
		}
	}
	return resource.Quantity{}, false
}

func snapshotNoSpaceFailurePolicy() *batchv1.PodFailurePolicy {
	containerName := CONTAINER_NAME_SNAPSHOT
	return &batchv1.PodFailurePolicy{
		Rules: []batchv1.PodFailurePolicyRule{
			{
				Action: batchv1.PodFailurePolicyActionFailJob,
				OnExitCodes: &batchv1.PodFailurePolicyOnExitCodesRequirement{
					ContainerName: &containerName,
					Operator:      batchv1.PodFailurePolicyOnExitCodesOpIn,
					Values:        []int32{snapshottool.ExitCodeNoSpace},
				},
			},
		},
	}
}

func SnapshotJobName(snap metav1.ObjectMeta, subCommand string) string {
	return fmt.Sprintf("snapshot-%s-%s", subCommand, snap.Name)
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
