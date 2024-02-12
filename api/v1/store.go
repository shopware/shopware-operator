package v1

import (
	autoscalerv2 "k8s.io/api/autoscaling/v2"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// Store is the Schema for the stores API
// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:printcolumn:name="State",type=string,JSONPath=".status.state"
// +kubebuilder:resource:scope=Namespaced
// +kubebuilder:resource:shortName=st
type Store struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`
	Spec              StoreSpec   `json:"spec,omitempty"`
	Status            StoreStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true
// StoreList contains a list of Store
type StoreList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []Store `json:"items"`
}

type StoreSpec struct {
	Database                DatabaseSpec  `json:"database"`
	Container               ContainerSpec `json:"container"`
	Network                 NetworkSpec   `json:"network,omitempty"`
	S3Storage               S3Storage     `json:"s3Storage,omitempty"`
	CDNURL                  string        `json:"cdnURL"`
	Blackfire               BlackfireSpec `json:"blackfire,omitempty"`
	Otel                    OtelSpec      `json:"otel,omitempty"`
	FPM                     FPMSpec       `json:"fpm,omitempty"`
	HorizontalPodAutoscaler HPASpec       `json:"horizontalPodAutoscaler,omitempty"`

	// +kubebuilder:default=false
	DisableChecks bool `json:"disableChecks,omitempty"`

	// +kubebuilder:default={adapter: "builtin"}
	SessionCache SessionCacheSpec `json:"sessionCache"`

	// +kubebuilder:default={adapter: "builtin"}
	AppCache AppCacheSpec `json:"appCache"`

	// +kubebuilder:default={adapter: "builtin"}
	Worker WorkerSpec `json:"worker"`

	// +kubebuilder:default=store-secret
	SecretName string `json:"secretName"`

	// +kubebuilder:default={username: "admin", password: ""}
	AdminCredentials Credentials `json:"adminCredentials"`

	SetupHook Hook `json:"setupHook,omitempty"`

	MigrationHook Hook `json:"migrationHook,omitempty"`
}

func init() {
	SchemeBuilder.Register(&Store{}, &StoreList{})
}

type Credentials struct {
	Username string `json:"username"`
	Password string `json:"password,omitempty"`
}

type Hook struct {
	After  string `json:"after"`
	Before string `json:"before"`
}

type HPASpec struct {
	MinReplicas *int32                                        `json:"minReplicas,omitempty" protobuf:"varint,2,opt,name=minReplicas"`
	MaxReplicas int32                                         `json:"maxReplicas" protobuf:"varint,3,opt,name=maxReplicas"`
	Metrics     []autoscalerv2.MetricSpec                     `json:"metrics,omitempty" protobuf:"bytes,4,rep,name=metrics"`
	Behavior    *autoscalerv2.HorizontalPodAutoscalerBehavior `json:"behavior,omitempty" protobuf:"bytes,5,opt,name=behavior"`

	// +kubebuilder:default=false
	Enabled     bool              `json:"enabled"`
	Annotations map[string]string `json:"annotations,omitempty"`
}

type NetworkSpec struct {
	// +kubebuilder:default=false
	EnabledIngress bool `json:"enabledIngress"`

	// +kubebuilder:validation:MinLength=1
	Host string `json:"host"`

	// +kubebuilder:default=8000
	Port int32 `json:"port,omitempty"`

	IngressClassName string            `json:"ingressClassName,omitempty"`
	Annotations      map[string]string `json:"annotations,omitempty"`
	Labels           map[string]string `json:"labels,omitempty"`
}

type ContainerSpec struct {
	// +kubebuilder:validation:MinLength=1
	Image string `json:"image"`
	// +kubebuilder:default=IfNotPresent
	ImagePullPolicy corev1.PullPolicy `json:"imagePullPolicy,omitempty"`

	Volumes []corev1.Volume `json:"volumes,omitempty"`

	// +kubebuilder:default=8000
	Port int32 `json:"port,omitempty"`

	VolumeMounts     []corev1.VolumeMount          `json:"volumeMounts,omitempty"`
	ImagePullSecrets []corev1.LocalObjectReference `json:"imagePullSecrets,omitempty"`
	RestartPolicy    corev1.RestartPolicy          `json:"restartPolicy,omitempty"`
	SecurityContext  *corev1.PodSecurityContext    `json:"podSecurityContext,omitempty"`
	ExtraContainers  []corev1.Container            `json:"extraContainers,omitempty"`

	// +kubebuilder:default=2
	Replicas int32 `json:"replicas,omitempty"`
	// +kubebuilder:default=30
	ProgressDeadlineSeconds int32 `json:"progressDeadlineSeconds,omitempty"`

	// StartupProbe   corev1.Probe `json:"startupProbe,omitempty"`
	// ReadinessProbe corev1.Probe `json:"readinessProbe,omitempty"`
	// LivenessProbe  corev1.Probe `json:"livenessProbe,omitempty"`

	NodeSelector              map[string]string                 `json:"nodeSelector,omitempty"`
	Annotations               map[string]string                 `json:"annotations,omitempty"`
	Labels                    map[string]string                 `json:"labels,omitempty"`
	TopologySpreadConstraints []corev1.TopologySpreadConstraint `json:"topologySpreadConstraints,omitempty"`
	Tolerations               []corev1.Toleration               `json:"tolerations,omitempty"`
	Affinity                  corev1.Affinity                   `json:"affinity,omitempty"`
	// InitImage   string            `json:"initImage,omitempty"`

	Resources corev1.ResourceRequirements `json:"resources,omitempty"`

	// PriorityClassName             string              `json:"priorityClassName,omitempty"`
	// TerminationGracePeriodSeconds *int64              `json:"gracePeriod,omitempty"`
	// SchedulerName                 string              `json:"schedulerName,omitempty"`
	// RuntimeClassName              *string             `json:"runtimeClassName,omitempty"`

	// ServiceAccountName string                     `json:"serviceAccountName,omitempty"`

	// Configuration string `json:"configuration,omitempty"`
	ExtraEnvs []corev1.EnvVar `json:"extraEnvs,omitempty"`
}

type SessionCacheSpec struct {
	RedisSpec `json:",inline"`

	// +kubebuilder:validation:Enum=builtin;redis
	Adapter  string `json:"adapter"`
	SavePath string `json:"savePath,omitempty"`
}

type WorkerSpec struct {
	RedisSpec `json:",inline"`

	// +kubebuilder:validation:Enum=builtin;redis
	Adapter string `json:"adapter"`
}

type AppCacheSpec struct {
	RedisSpec `json:",inline"`

	// +kubebuilder:validation:Enum=builtin;redis
	Adapter string `json:"adapter"`
}

type RedisSpec struct {
	RedisHost string `json:"redisHost,omitempty"`
	// +kubebuilder:default=6379
	RedisPort int `json:"redisPort,omitempty"`
	// +kubebuilder:default=0
	RedisIndex int `json:"redisDatabase,omitempty"`
}

type FPMSpec struct {
	// +kubebuilder:validation:Enum=static;dynamic;ondemand
	// +kubebuilder:default=static
	ProcessManagement string `json:"processManagement"`

	// +kubebuilder:default="127.0.0.1:9000"
	Listen string `json:"listen"`
	// +kubebuilder:default="tcp://127.0.0.1:9000/status"
	ScrapeURI string `json:"scrapeURI"`
	// +kubebuilder:default=/status
	StatusPath string `json:"statusPath"`

	// +kubebuilder:default=8
	MaxChildren int `json:"maxChildren"`
	// +kubebuilder:default=8
	StartServers int `json:"startServers"`
	// +kubebuilder:default=4
	MinSpareServers int `json:"minSpareServers"`
	// +kubebuilder:default=8
	MaxSpareServers int `json:"maxSpareServers"`
}

type BlackfireSpec struct {
	// +kubebuilder:default=false
	Enabled bool `json:"enabled,omitempty"`

	// +kubebuilder:default=blackfire
	Host string `json:"host,omitempty"`
	// +kubebuilder:default=8307
	Port int `json:"port,omitempty"`
}

type OtelSpec struct {
	// +kubebuilder:default=false
	Enabled bool `json:"enabled"`

	// +kubebuilder:default=shopware
	ServiceName string `json:"serviceName,omitempty"`
	// +kubebuilder:default=otlp
	TracesExporter string `json:"tracesExporter,omitempty"`
	// +kubebuilder:default=grpc
	ExporterProtocol string `json:"exporterProtocol,omitempty"`

	ExporterEndpoint string `json:"exporterEndpoint,omitempty"`
}

type S3Storage struct {
	// +kubebuilder:validation:MinLength=1
	EndpointURL string `json:"endpointURL"`
	// +kubebuilder:validation:MinLength=1
	PrivateBucketName string `json:"privateBucketName"`
	// +kubebuilder:validation:MinLength=1
	PublicBucketName string `json:"publicBucketName"`
	Region           string `json:"region,omitempty"`

	AccessKeyRef       SecretRef `json:"accessKeyRef"`
	SecretAccessKeyRef SecretRef `json:"secretAccessKeyRef"`
}

type DatabaseSpec struct {
	// +kubebuilder:validation:MinLength=1
	Host string `json:"host"`
	// +kubebuilder:default=3306
	Port int32 `json:"port"`
	// +kubebuilder:validation:MinLength=1
	Version string `json:"version"`
	// +kubebuilder:validation:MinLength=1
	User string `json:"user"`
	// +kubebuilder:validation:MinLength=1
	// +kubebuilder:default=shopware
	Name string `json:"name"`

	PasswordSecretRef SecretRef `json:"passwordSecretRef"`
}

type SecretRef struct {
	// +kubebuilder:validation:MinLength=1
	Name string `json:"name"`
	// +kubebuilder:validation:MinLength=1
	Key string `json:"key"`
}

func (s *Store) GetSecretName() string {
	return s.Spec.SecretName
}
