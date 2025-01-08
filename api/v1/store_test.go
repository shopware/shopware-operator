package v1_test

import (
	"testing"

	v1 "github.com/shopware/shopware-operator/api/v1"
	"github.com/stretchr/testify/assert"
)

func TestStoreContainer(t *testing.T) {
	con := &v1.Store{
		Spec: v1.StoreSpec{
			Container: v1.ContainerSpec{
				Image:    "FirstContainer",
				Replicas: 2,
				Labels: map[string]string{
					"test":  "FirstContainer",
					"test2": "FirstContainer",
				},
				Annotations: map[string]string{
					"test":  "FirstContainer",
					"test2": "FirstContainer",
				},
			},
		},
	}

	con2 := v1.ContainerMergeSpec{
		Image: "SecondContainer",
		Labels: map[string]string{
			"test":  "SecondContainer",
			"test2": "SecondContainer",
		},
		Annotations: map[string]string{
			"test": "SecondContainer",
		},
	}

	con.Spec.Container.Merge(con2)
	assert.Equal(t, "SecondContainer", con.Spec.Container.Image)
	assert.Equal(t, int32(2), con.Spec.Container.Replicas)
	assert.Equal(t, "SecondContainer", con.Spec.Container.Labels["test"])
	assert.Equal(t, "SecondContainer", con.Spec.Container.Labels["test2"])
	assert.Equal(t, "SecondContainer", con.Spec.Container.Annotations["test"])
	assert.Equal(t, "FirstContainer", con.Spec.Container.Annotations["test2"])
}
