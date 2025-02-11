package v1_test

import (
	"testing"

	v1 "github.com/shopware/shopware-operator/api/v1"
	"github.com/shopware/shopware-operator/internal/util"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
)

// Helper function to create a base container spec with common fields
func createTestContainerSpec(name string, replicas int32, policy corev1.PullPolicy) v1.ContainerSpec {
	return v1.ContainerSpec{
		Image:                   name,
		ImagePullPolicy:         policy,
		Replicas:                replicas,
		ProgressDeadlineSeconds: 30,
		RestartPolicy:           corev1.RestartPolicyAlways,
		ExtraEnvs: []corev1.EnvVar{
			{Name: "ENV1", Value: "value1"},
		},
		VolumeMounts: []corev1.VolumeMount{
			{Name: "vol1", MountPath: "/path1"},
		},
		ImagePullSecrets: []corev1.LocalObjectReference{
			{Name: "secret1"},
		},
		Volumes: []corev1.Volume{
			{Name: "vol1"},
		},
		Resources: corev1.ResourceRequirements{
			Limits: corev1.ResourceList{
				corev1.ResourceCPU: resource.MustParse("1"),
			},
			Requests: corev1.ResourceList{
				corev1.ResourceMemory: resource.MustParse("1Gi"),
			},
		},
		ExtraContainers: []corev1.Container{
			{Name: "sidecar1"},
		},
		NodeSelector: map[string]string{
			"node": "type1",
		},
		TopologySpreadConstraints: []corev1.TopologySpreadConstraint{
			{TopologyKey: "zone"},
		},
		Tolerations: []corev1.Toleration{
			{Key: "key1"},
		},
		Labels: map[string]string{
			"test": name,
		},
		Annotations: map[string]string{
			"test": name,
		},
		TerminationGracePeriodSeconds: 10,
	}
}

func TestStoreContainer(t *testing.T) {
	baseContainer := &v1.Store{
		Spec: v1.StoreSpec{
			Container: createTestContainerSpec("FirstContainer", 2, corev1.PullAlways),
		},
	}

	mergeSpec := createMergeSpec("SecondContainer", 3, corev1.PullIfNotPresent)
	baseContainer.Spec.Container.Merge(mergeSpec)

	// Verify all merged properties
	assert.Equal(t, "SecondContainer", baseContainer.Spec.Container.Image)
	assert.Equal(t, corev1.PullIfNotPresent, baseContainer.Spec.Container.ImagePullPolicy)
	assert.Equal(t, int32(3), baseContainer.Spec.Container.Replicas)
	assert.Equal(t, int32(60), baseContainer.Spec.Container.ProgressDeadlineSeconds)
	assert.Equal(t, corev1.RestartPolicyNever, baseContainer.Spec.Container.RestartPolicy)
	assert.Equal(t, []corev1.EnvVar{{Name: "ENV2", Value: "value2"}}, baseContainer.Spec.Container.ExtraEnvs)
	assert.Equal(t, []corev1.VolumeMount{{Name: "vol2", MountPath: "/path2"}}, baseContainer.Spec.Container.VolumeMounts)
	assert.Equal(t, []corev1.LocalObjectReference{{Name: "secret2"}}, baseContainer.Spec.Container.ImagePullSecrets)
	assert.Equal(t, []corev1.Volume{{Name: "vol2"}}, baseContainer.Spec.Container.Volumes)
	assert.Equal(t, resource.MustParse("2"), baseContainer.Spec.Container.Resources.Limits[corev1.ResourceCPU])
	assert.Equal(t, resource.MustParse("2Gi"), baseContainer.Spec.Container.Resources.Requests[corev1.ResourceMemory])
	assert.Equal(t, []corev1.Container{{Name: "sidecar2"}}, baseContainer.Spec.Container.ExtraContainers)
	assert.Equal(t, map[string]string{"node": "type2"}, baseContainer.Spec.Container.NodeSelector)
	assert.Equal(t, []corev1.TopologySpreadConstraint{{TopologyKey: "region"}}, baseContainer.Spec.Container.TopologySpreadConstraints)
	assert.Equal(t, []corev1.Toleration{{Key: "key2"}}, baseContainer.Spec.Container.Tolerations)
	assert.Equal(t, "SecondContainer", baseContainer.Spec.Container.Labels["test"])
	assert.Equal(t, "SecondContainer", baseContainer.Spec.Container.Annotations["test"])
	assert.Equal(t, int64(20), baseContainer.Spec.Container.TerminationGracePeriodSeconds)
}

// Helper function to create a merge spec
func createMergeSpec(name string, replicas int32, policy corev1.PullPolicy) v1.ContainerMergeSpec {
	return v1.ContainerMergeSpec{
		Image:                   name,
		ImagePullPolicy:         policy,
		Replicas:                replicas,
		ProgressDeadlineSeconds: 60,
		RestartPolicy:           corev1.RestartPolicyNever,
		ExtraEnvs: []corev1.EnvVar{
			{Name: "ENV2", Value: "value2"},
		},
		VolumeMounts: []corev1.VolumeMount{
			{Name: "vol2", MountPath: "/path2"},
		},
		ImagePullSecrets: []corev1.LocalObjectReference{
			{Name: "secret2"},
		},
		Volumes: []corev1.Volume{
			{Name: "vol2"},
		},
		Resources: corev1.ResourceRequirements{
			Limits: corev1.ResourceList{
				corev1.ResourceCPU: resource.MustParse("2"),
			},
			Requests: corev1.ResourceList{
				corev1.ResourceMemory: resource.MustParse("2Gi"),
			},
		},
		ExtraContainers: []corev1.Container{
			{Name: "sidecar2"},
		},
		NodeSelector: map[string]string{
			"node": "type2",
		},
		TopologySpreadConstraints: []corev1.TopologySpreadConstraint{
			{TopologyKey: "region"},
		},
		Tolerations: []corev1.Toleration{
			{Key: "key2"},
		},
		Labels: map[string]string{
			"test": name,
		},
		Annotations: map[string]string{
			"test": name,
		},
		TerminationGracePeriodSeconds: 20,
	}
}

func TestStoreContainerEmptyMerge(t *testing.T) {
	baseContainer := &v1.Store{
		Spec: v1.StoreSpec{
			Container: v1.ContainerSpec{
				Image:                   "originalImage",
				ImagePullPolicy:         corev1.PullAlways,
				Replicas:                5,
				ProgressDeadlineSeconds: 30,
				RestartPolicy:           corev1.RestartPolicyAlways,
				ExtraEnvs: []corev1.EnvVar{
					{Name: "ORIGINAL", Value: "value"},
				},
				VolumeMounts: []corev1.VolumeMount{
					{Name: "original-volume", MountPath: "/original"},
				},
				Resources: corev1.ResourceRequirements{
					Limits: corev1.ResourceList{
						corev1.ResourceCPU: resource.MustParse("1"),
					},
				},
				Labels: map[string]string{
					"original": "value",
				},
				Annotations: map[string]string{
					"original": "value",
				},
				TerminationGracePeriodSeconds: 10,
			},
		},
	}

	// Create merge spec with empty/zero values
	emptyMergeSpec := v1.ContainerMergeSpec{}

	// Perform merge
	baseContainer.Spec.Container.Merge(emptyMergeSpec)

	// Verify that original values are retained
	assert.Equal(t, "originalImage", baseContainer.Spec.Container.Image)
	assert.Equal(t, corev1.PullAlways, baseContainer.Spec.Container.ImagePullPolicy)
	assert.Equal(t, int32(5), baseContainer.Spec.Container.Replicas)
	assert.Equal(t, int32(30), baseContainer.Spec.Container.ProgressDeadlineSeconds)
	assert.Equal(t, corev1.RestartPolicyAlways, baseContainer.Spec.Container.RestartPolicy)
	assert.Equal(t, []corev1.EnvVar{{Name: "ORIGINAL", Value: "value"}}, baseContainer.Spec.Container.ExtraEnvs)
	assert.Equal(t, []corev1.VolumeMount{{Name: "original-volume", MountPath: "/original"}}, baseContainer.Spec.Container.VolumeMounts)
	assert.Equal(t, resource.MustParse("1"), baseContainer.Spec.Container.Resources.Limits[corev1.ResourceCPU])
	assert.Equal(t, "value", baseContainer.Spec.Container.Labels["original"])
	assert.Equal(t, "value", baseContainer.Spec.Container.Annotations["original"])
	assert.Equal(t, int64(10), baseContainer.Spec.Container.TerminationGracePeriodSeconds)

	// Verify that nil slices/maps stayed nil or empty
	assert.Nil(t, emptyMergeSpec.ImagePullSecrets)
	assert.Nil(t, emptyMergeSpec.Volumes)
	assert.Nil(t, emptyMergeSpec.ExtraContainers)
	assert.Nil(t, emptyMergeSpec.TopologySpreadConstraints)
	assert.Nil(t, emptyMergeSpec.Tolerations)
}

func TestServiceAccountMerge(t *testing.T) {
	baseContainer := &v1.Store{
		Spec: v1.StoreSpec{
			Container: v1.ContainerSpec{
				ServiceAccountName: "original-sa",
			},
		},
	}

	// Test 1: Merging with a new service account
	mergeSpec := v1.ContainerMergeSpec{
		ServiceAccountName: "new-sa",
	}
	baseContainer.Spec.Container.Merge(mergeSpec)
	assert.Equal(t, "new-sa", baseContainer.Spec.Container.ServiceAccountName)

	// Test 2: Merging with empty service account (should not override)
	emptyMergeSpec := v1.ContainerMergeSpec{}
	baseContainer.Spec.Container.Merge(emptyMergeSpec)
	assert.Equal(t, "new-sa", baseContainer.Spec.Container.ServiceAccountName)
}

func TestSecurityContextMerge(t *testing.T) {
	baseContainer := &v1.Store{
		Spec: v1.StoreSpec{
			Container: v1.ContainerSpec{
				SecurityContext: &corev1.PodSecurityContext{
					RunAsUser: util.Int64(1000),
				},
			},
		},
	}

	// Test 1: Complete override of security context
	mergeSpec := v1.ContainerMergeSpec{
		SecurityContext: &corev1.PodSecurityContext{
			RunAsGroup: util.Int64(2000),
		},
	}
	baseContainer.Spec.Container.Merge(mergeSpec)
	assert.Equal(t, int64(2000), *baseContainer.Spec.Container.SecurityContext.RunAsGroup)
	assert.Nil(t, baseContainer.Spec.Container.SecurityContext.RunAsUser)

	// Test 2: Merging with nil security context (should not override)
	emptyMergeSpec := v1.ContainerMergeSpec{}
	baseContainer.Spec.Container.Merge(emptyMergeSpec)
	assert.NotNil(t, baseContainer.Spec.Container.SecurityContext)
	assert.Equal(t, int64(2000), *baseContainer.Spec.Container.SecurityContext.RunAsGroup)
}

func TestAffinity(t *testing.T) {
	baseContainer := &v1.Store{
		Spec: v1.StoreSpec{
			Container: v1.ContainerSpec{
				Affinity: corev1.Affinity{
					NodeAffinity: &corev1.NodeAffinity{
						RequiredDuringSchedulingIgnoredDuringExecution: &corev1.NodeSelector{
							NodeSelectorTerms: []corev1.NodeSelectorTerm{
								{
									MatchExpressions: []corev1.NodeSelectorRequirement{
										{
											Key: "original",
										},
									},
								},
							},
						},
					},
				},
			},
		},
	}

	// Test affinity override
	mergeSpec := v1.ContainerMergeSpec{
		Affinity: corev1.Affinity{
			NodeAffinity: &corev1.NodeAffinity{
				RequiredDuringSchedulingIgnoredDuringExecution: &corev1.NodeSelector{
					NodeSelectorTerms: []corev1.NodeSelectorTerm{
						{
							MatchExpressions: []corev1.NodeSelectorRequirement{
								{
									Key: "merged",
								},
							},
						},
					},
				},
			},
		},
	}

	baseContainer.Spec.Container.Merge(mergeSpec)
	assert.Equal(t, "merged", baseContainer.Spec.Container.Affinity.NodeAffinity.RequiredDuringSchedulingIgnoredDuringExecution.NodeSelectorTerms[0].MatchExpressions[0].Key)
}

func TestEnvMerge(t *testing.T) {
	baseContainer := &v1.Store{
		Spec: v1.StoreSpec{
			Container: v1.ContainerSpec{
				Image:                   "originalImage",
				ImagePullPolicy:         corev1.PullAlways,
				Replicas:                5,
				ProgressDeadlineSeconds: 30,
				RestartPolicy:           corev1.RestartPolicyAlways,
				ExtraEnvs: []corev1.EnvVar{
					{Name: "APP_URL", Value: "overwritten"},
					{Name: "NEW", Value: "exists"},
				},
				VolumeMounts: []corev1.VolumeMount{
					{Name: "original-volume", MountPath: "/original"},
				},
				Resources: corev1.ResourceRequirements{
					Limits: corev1.ResourceList{
						corev1.ResourceCPU: resource.MustParse("1"),
					},
				},
				Labels: map[string]string{
					"original": "value",
				},
				Annotations: map[string]string{
					"original": "value",
				},
				TerminationGracePeriodSeconds: 10,
			},
		},
	}
	env := baseContainer.GetEnv()
	for _, envVar := range env {
		if envVar.Name == "APP_URL" {
			require.Equal(t, "overwritten", envVar.Value)
		}
		if envVar.Name == "NEW" {
			require.Equal(t, "exists", envVar.Value)
		}
	}
}
