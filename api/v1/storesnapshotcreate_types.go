package v1

import (
	"slices"
	"strings"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

type S3BucketAuth struct {
	AccessKeyRef       SecretRef `json:"accessKeyRef,omitempty"`
	SecretAccessKeyRef SecretRef `json:"secretAccessKeyRef,omitempty"`
}

type (
	SnapshotState string
)

type StoreSnapshotSpec struct {
	StoreNameRef string `json:"storeNameRef"`

	// +kubebuilder:validation:Pattern="^(s3://[a-z0-9.-]+(/[a-zA-Z0-9._-]+)*)|(\\.?(/[^/ ]+)+)$"
	// +kubebuilder:validation:Required
	// +kubebuilder:example="s3://my-bucket-name/optional/path"
	// +kubebuilder:example="/optional/local/path"
	// +kubebuilder:description="The destination path where the snapshot will be stored. This can be either a local filesystem path (e.g., /backups/snapshot.zip) or an S3 URI (e.g., s3://my-bucket-name/optional/backup.zip)."
	Path string `json:"path"`

	// +kubebuilder:description="Used for access to the s3 buckets of the shop and also used when storing to a snapshot to s3"
	S3BucketAuth S3BucketAuth `json:"s3BucketAuth,omitempty"`

	// +kubebuilder:default=3
	MaxRetries int32 `json:"maxRetries,omitempty"`

	Container ContainerSpec `json:"container"`
}

type StoreSnapshotStatus struct {
	State       SnapshotState `json:"state,omitempty"`
	CompletedAt metav1.Time   `json:"completed,omitempty"`
	Message     string        `json:"message,omitempty"`
}

const (
	SnapshotStateEmpty     SnapshotState = ""
	SnapshotStatePending   SnapshotState = "pending"
	SnapshotStateRunning   SnapshotState = "running"
	SnapshotStateFailed    SnapshotState = "failed"
	SnapshotStateSucceeded SnapshotState = "succeeded"
)

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:resource:scope=Namespaced
// +kubebuilder:resource:shortName=store-snap-create
// +kubebuilder:printcolumn:name="State",type=string,JSONPath=".status.state"
// +kubebuilder:printcolumn:name="Message",type=string,JSONPath=".status.message"
// +kubebuilder:printcolumn:name="Completed",type="date",JSONPath=".status.completed"
// +kubebuilder:printcolumn:name="Age",type="date",JSONPath=".metadata.creationTimestamp"

type StoreSnapshotCreate struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   StoreSnapshotSpec   `json:"spec,omitempty"`
	Status StoreSnapshotStatus `json:"status,omitempty"`
}

//+kubebuilder:object:root=true

type StoreSnapshotCreateList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []StoreSnapshotCreate `json:"items"`
}

func init() {
	SchemeBuilder.Register(&StoreSnapshotCreate{}, &StoreSnapshotCreateList{})
}

func (s *StoreSnapshotStatus) IsState(states ...SnapshotState) bool {
	return slices.Contains(states, s.State)
}

func (s StoreSnapshotSpec) GetEnv(store Store) []corev1.EnvVar {
	env := []corev1.EnvVar{
		{
			Name: "DB_HOST",
			ValueFrom: &corev1.EnvVarSource{
				SecretKeyRef: &corev1.SecretKeySelector{
					LocalObjectReference: corev1.LocalObjectReference{
						Name: store.Spec.SecretName,
					},
					Key: "database-host",
				},
			},
		},
		{
			Name: "DB_PASSWORD",
			ValueFrom: &corev1.EnvVarSource{
				SecretKeyRef: &corev1.SecretKeySelector{
					LocalObjectReference: corev1.LocalObjectReference{
						Name: store.Spec.SecretName,
					},
					Key: "database-password",
				},
			},
		},
		{
			Name: "DB_USER",
			ValueFrom: &corev1.EnvVarSource{
				SecretKeyRef: &corev1.SecretKeySelector{
					LocalObjectReference: corev1.LocalObjectReference{
						Name: store.Spec.SecretName,
					},
					Key: "database-user",
				},
			},
		},
		{
			Name:  "DB_DATABASE",
			Value: store.Spec.Database.Name,
		},
		{
			Name:  "AWS_ENDPOINT",
			Value: strings.ReplaceAll(store.Spec.S3Storage.EndpointURL, "https://", ""),
		},
		{
			Name:  "AWS_PRIVATE_BUCKET",
			Value: store.Spec.S3Storage.PrivateBucketName,
		},
		{
			Name:  "AWS_PUBLIC_BUCKET",
			Value: store.Spec.S3Storage.PublicBucketName,
		},
	}

	if s.S3BucketAuth.AccessKeyRef.Name != "" {
		env = append(env, corev1.EnvVar{
			Name: "AWS_ACCESS_KEY_ID",
			ValueFrom: &corev1.EnvVarSource{
				SecretKeyRef: &corev1.SecretKeySelector{
					LocalObjectReference: corev1.LocalObjectReference{
						Name: s.S3BucketAuth.AccessKeyRef.Name,
					},
					Key: s.S3BucketAuth.AccessKeyRef.Key,
				},
			},
		})
	}

	if s.S3BucketAuth.AccessKeyRef.Name != "" {
		env = append(env, corev1.EnvVar{
			Name: "AWS_SECRET_ACCESS_KEY",
			ValueFrom: &corev1.EnvVarSource{
				SecretKeyRef: &corev1.SecretKeySelector{
					LocalObjectReference: corev1.LocalObjectReference{
						Name: s.S3BucketAuth.SecretAccessKeyRef.Name,
					},
					Key: s.S3BucketAuth.SecretAccessKeyRef.Key,
				},
			},
		})
	}

	return env
}
