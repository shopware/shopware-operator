package v1

import (
	"maps"

	autoscalerv2 "k8s.io/api/autoscaling/v2"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// Store is the Schema for the stores API
// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:printcolumn:name="Storefront",type=string,JSONPath=".status.storefrontState.ready"
// +kubebuilder:printcolumn:name="Worker",type=string,JSONPath=".status.workerState.ready"
// +kubebuilder:printcolumn:name="Admin",type=string,JSONPath=".status.adminState.ready"
// +kubebuilder:printcolumn:name="State",type=string,JSONPath=".status.state"
// +kubebuilder:printcolumn:name="Message",type=string,JSONPath=".status.message"
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
	Database DatabaseSpec `json:"database"`

	Container ContainerSpec `json:"container"`

	// +kubebuilder:default={}
	AdminDeploymentContainer ContainerMergeSpec `json:"adminDeploymentContainer,omitempty"`
	// +kubebuilder:default={}
	WorkerDeploymentContainer ContainerMergeSpec `json:"workerDeploymentContainer,omitempty"`
	// +kubebuilder:default={}
	StorefrontDeploymentContainer ContainerMergeSpec `json:"storefrontDeploymentContainer,omitempty"`
	// +kubebuilder:default={}
	SetupJobContainer ContainerMergeSpec `json:"setupJobContainer,omitempty"`
	// +kubebuilder:default={}
	MigrationJobContainer ContainerMergeSpec `json:"migrationJobContainer,omitempty"`

	Network                 NetworkSpec   `json:"network,omitempty"`
	S3Storage               S3Storage     `json:"s3Storage,omitempty"`
	CDNURL                  string        `json:"cdnURL"`
	Blackfire               BlackfireSpec `json:"blackfire,omitempty"`
	Otel                    OtelSpec      `json:"otel,omitempty"`
	FPM                     FPMSpec       `json:"fpm,omitempty"`
	HorizontalPodAutoscaler HPASpec       `json:"horizontalPodAutoscaler,omitempty"`

	// +kubebuilder:default={enabled: false}
	OpensearchSpec OpensearchSpec `json:"opensearch,omitempty"`

	// Use ServiceAccountName in container spec
	ServiceAccountName string `json:"serviceAccountName,omitempty"`

	// +kubebuilder:default={currency: "EUR", locale: "en-GB", usageDataConsent: "allowed"}
	ShopConfiguration Configuration `json:"shopConfiguration,omitempty"`

	// +kubebuilder:default=false
	DisableChecks bool `json:"disableChecks,omitempty"`
	// +kubebuilder:default=false
	DisableS3Check bool `json:"disableS3Check,omitempty"`
	// +kubebuilder:default=false
	DisableFastlyCheck bool `json:"disableFastlyCheck,omitempty"`
	// +kubebuilder:default=false
	DisableOpensearchCheck bool `json:"disableOpensearchCheck,omitempty"`
	// +kubebuilder:default=false
	DisableDatabaseCheck bool `json:"disableDatabaseCheck,omitempty"`
	DisableJobDeletion   bool `json:"disableJobDeletion,omitempty"`

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

	//+kubebuilder:deprecatedversion
	SetupHook Hook `json:"setupHook,omitempty"`
	// +kubebuilder:default=/setup
	SetupScript string `json:"setupScript,omitempty"`

	//+kubebuilder:deprecatedversion
	MigrationHook Hook `json:"migrationHook,omitempty"`
	// +kubebuilder:default=/setup
	MigrationScript string `json:"migrationScript,omitempty"`

	// +kubebuilder:default={timeZone: "Etc/UTC", schedule: "0 * * * *", command: "bin/console scheduled-task:run -v -n --no-wait"}
	ScheduledTask ScheduledTaskSpec `json:"scheduledTask,omitempty"`
}

func init() {
	SchemeBuilder.Register(&Store{}, &StoreList{})
}

type Configuration struct {
	// +kubebuilder:default="EUR"
	Currency string `json:"currency"`
	// +kubebuilder:default="en-GB"
	Locale string     `json:"locale"`
	Fastly FastlySpec `json:"fastly,omitempty"`

	// +kubebuilder:validation:Enum=revoked;allowed
	// +kubebuilder:default=allowed
	UsageDataConsent string `json:"usageDataConsent,omitempty"`
}

type FastlySpec struct {
	TokenRef   SecretRef `json:"tokenRef,omitempty"`
	ServiceRef SecretRef `json:"serviceRef,omitempty"`
}

type Credentials struct {
	Username string `json:"username"`
	Password string `json:"password,omitempty"`
}

type Hook struct {
	After  string `json:"after,omitempty"`
	Before string `json:"before,omitempty"`
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

type ScheduledTaskSpec struct {
	// The time zone name for the given schedule, see https://en.wikipedia.org/wiki/List_of_tz_database_time_zones. Default is Etc/UTC.
	// +kubebuilder:default="Etc/UTC"
	TimeZone string `json:"timeZone"`

	// The schedule in Cron format, see https://en.wikipedia.org/wiki/Cron.
	// Default to every hour at minute 0.
	// +kubebuilder:default="0 * * * *"
	Schedule string `json:"schedule"`

	// This flag tells the controller to suspend subsequent executions, it does
	// not apply to already started executions.  Defaults to false.
	// +kubebuilder:default=false
	Suspend bool `json:"suspend,omitempty"`

	// The schedule task command
	// +kubebuilder:default="bin/console scheduled-task:run -v -n --no-wait"
	Command string `json:"command"`
}

type OpensearchSpec struct {
	Enabled bool `json:"enabled"`

	Host              string    `json:"host,omitempty"`
	Username          string    `json:"username,omitempty"`
	PasswordSecretRef SecretRef `json:"passwordSecretRef,omitempty"`

	// +kubebuilder:default=9200
	Port int32 `json:"port,omitempty"`

	// +kubebuilder:validation:Enum=http;https
	// +kubebuilder:default=https
	Schema string `json:"schema,omitempty"`

	// +kubebuilder:default={prefix: sw, shards: 3, replicas: 3}
	Index OpensearchIndexSpec `json:"index,omitempty"`
}

type OpensearchIndexSpec struct {
	Prefix   string `json:"prefix,omitempty"`
	Shards   int    `json:"shards,omitempty"`
	Replicas int    `json:"replicas,omitempty"`
}

type NetworkSpec struct {
	// +kubebuilder:default=false
	EnabledIngress bool `json:"enabledIngress"`

	// +kubebuilder:default=false
	EnabledGateway bool `json:"enabledGateway"`

	// +kubebuilder:validation:deprecatedversion
	Host string `json:"host,omitempty"`

	// The listType is used to make the slice unique, the unique constraint is not working: the runtime complexity becomes quadratic
	Hosts      []string `json:"hosts,omitempty"`
	AppURLHost string   `json:"appURLHost,omitempty"`

	// +kubebuilder:default=8000
	Port int32 `json:"port,omitempty"`

	// +kubebuilder:description="Only for Ingress, but use ingressAnnotations to set annotations"
	// +kubebuilder:validation:deprecatedversion
	Annotations map[string]string `json:"annotations,omitempty"`
	// +kubebuilder:description="Only for Ingress, but use ingressLabels to set labels"
	// +kubebuilder:validation:deprecatedversion
	Labels map[string]string `json:"labels,omitempty"`

	IngressClassName   string            `json:"ingressClassName,omitempty"`
	IngressAnnotations map[string]string `json:"ingressAnnotations,omitempty"`
	IngressLabels      map[string]string `json:"ingressLabels,omitempty"`

	GatewayName        string            `json:"gatewayName,omitempty"`
	GatewayNamespace   string            `json:"gatewayNamespace,omitempty"`
	GatewaySectionName string            `json:"gatewaySectionName,omitempty"`
	GatewayAnnotations map[string]string `json:"gatewayAnnotations,omitempty"`
	GatewayLabels      map[string]string `json:"gatewayLabels,omitempty"`

	// +kubebuilder:default=store-tls
	TLSSecretName string `json:"tlsSecretName,omitempty"`
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
	InitContainers   []corev1.Container            `json:"initContainers,omitempty"`

	// +kubebuilder:default=2
	Replicas int32 `json:"replicas,omitempty"`
	// +kubebuilder:default=30
	ProgressDeadlineSeconds int32 `json:"progressDeadlineSeconds,omitempty"`

	// +kubebuilder:default=30
	TerminationGracePeriodSeconds int64 `json:"terminationGracePeriodSeconds,omitempty"`

	// StartupProbe   corev1.Probe `json:"startupProbe,omitempty"`
	// ReadinessProbe corev1.Probe `json:"readinessProbe,omitempty"`
	// LivenessProbe  corev1.Probe `json:"livenessProbe,omitempty"`

	Annotations               map[string]string                 `json:"annotations,omitempty"`
	Labels                    map[string]string                 `json:"labels,omitempty"`
	NodeSelector              map[string]string                 `json:"nodeSelector,omitempty"`
	TopologySpreadConstraints []corev1.TopologySpreadConstraint `json:"topologySpreadConstraints,omitempty"`
	Tolerations               []corev1.Toleration               `json:"tolerations,omitempty"`
	Affinity                  corev1.Affinity                   `json:"affinity,omitempty"`
	// InitImage   string            `json:"initImage,omitempty"`

	Resources corev1.ResourceRequirements `json:"resources,omitempty"`

	// PriorityClassName             string              `json:"priorityClassName,omitempty"`
	// TerminationGracePeriodSeconds *int64              `json:"gracePeriod,omitempty"`
	// SchedulerName                 string              `json:"schedulerName,omitempty"`
	// RuntimeClassName              *string             `json:"runtimeClassName,omitempty"`

	ServiceAccountName string `json:"serviceAccountName,omitempty"`

	// Configuration string `json:"configuration,omitempty"`
	ExtraEnvs []corev1.EnvVar `json:"extraEnvs,omitempty"`
}

type ContainerMergeSpec struct {
	// +kubebuilder:validation:MinLength=1
	Image                         string                        `json:"image,omitempty"`
	ImagePullPolicy               corev1.PullPolicy             `json:"imagePullPolicy,omitempty"`
	ImagePullSecrets              []corev1.LocalObjectReference `json:"imagePullSecrets,omitempty"`
	Volumes                       []corev1.Volume               `json:"volumes,omitempty"`
	VolumeMounts                  []corev1.VolumeMount          `json:"volumeMounts,omitempty"`
	RestartPolicy                 corev1.RestartPolicy          `json:"restartPolicy,omitempty"`
	SecurityContext               *corev1.PodSecurityContext    `json:"podSecurityContext,omitempty"`
	ExtraContainers               []corev1.Container            `json:"extraContainers,omitempty"`
	InitContainers                []corev1.Container            `json:"initContainers,omitempty"`
	Replicas                      int32                         `json:"replicas,omitempty"`
	ProgressDeadlineSeconds       int32                         `json:"progressDeadlineSeconds,omitempty"`
	TerminationGracePeriodSeconds int64                         `json:"terminationGracePeriodSeconds,omitempty"`

	Annotations               map[string]string                 `json:"annotations,omitempty"`
	Labels                    map[string]string                 `json:"labels,omitempty"`
	NodeSelector              map[string]string                 `json:"nodeSelector,omitempty"`
	TopologySpreadConstraints []corev1.TopologySpreadConstraint `json:"topologySpreadConstraints,omitempty"`
	Tolerations               []corev1.Toleration               `json:"tolerations,omitempty"`
	Affinity                  corev1.Affinity                   `json:"affinity,omitempty"`

	Resources          corev1.ResourceRequirements `json:"resources,omitempty"`
	ExtraEnvs          []corev1.EnvVar             `json:"extraEnvs,omitempty"`
	ServiceAccountName string                      `json:"serviceAccountName,omitempty"`
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
	RedisDSN  string `json:"redisDsn,omitempty"`
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

	AccessKeyRef       SecretRef `json:"accessKeyRef,omitempty"`
	SecretAccessKeyRef SecretRef `json:"secretAccessKeyRef,omitempty"`
}

type DatabaseSpec struct {
	Host    string    `json:"host,omitempty"`
	HostRef SecretRef `json:"hostRef,omitempty"`
	// +kubebuilder:default=3306
	Port int32 `json:"port"`
	// +kubebuilder:validation:MinLength=1
	Version string `json:"version"`
	// +kubebuilder:validation:MinLength=1
	User string `json:"user"`
	// +kubebuilder:validation:MinLength=1
	// +kubebuilder:default=shopware
	Name string `json:"name"`

	// +kubebuilder:validation:MinLength=1
	// +kubebuilder:default=PREFERRED
	SSLMode string `json:"sslMode,omitempty"`

	// +kubebuilder:example=?attribute1=value1&attribute2=value2...
	Options string `json:"options,omitempty"`

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

//nolint:gocyclo
func (c *ContainerSpec) Merge(from ContainerMergeSpec) {
	if from.Image != "" {
		c.Image = from.Image
	}
	if from.ImagePullPolicy != "" {
		c.ImagePullPolicy = from.ImagePullPolicy
	}
	if from.Replicas != 0 {
		c.Replicas = from.Replicas
	}
	if from.ProgressDeadlineSeconds != 0 {
		c.ProgressDeadlineSeconds = from.ProgressDeadlineSeconds
	}
	if from.RestartPolicy != "" {
		c.RestartPolicy = from.RestartPolicy
	}
	if from.ExtraEnvs != nil {
		c.ExtraEnvs = from.ExtraEnvs
	}
	if from.VolumeMounts != nil {
		c.VolumeMounts = from.VolumeMounts
	}
	if from.ImagePullSecrets != nil {
		c.ImagePullSecrets = from.ImagePullSecrets
	}
	if from.Volumes != nil {
		c.Volumes = from.Volumes
	}

	if from.ServiceAccountName != "" {
		c.ServiceAccountName = from.ServiceAccountName
	}

	// Initialize resources maps if nil
	if c.Resources.Requests == nil {
		c.Resources.Requests = make(corev1.ResourceList)
	}
	if c.Resources.Limits == nil {
		c.Resources.Limits = make(corev1.ResourceList)
	}

	// Always copy existing resources first
	if from.Resources.Requests != nil {
		for k, v := range from.Resources.Requests {
			c.Resources.Requests[k] = v
		}
	}
	if from.Resources.Limits != nil {
		for k, v := range from.Resources.Limits {
			c.Resources.Limits[k] = v
		}
	}

	// Handle security context
	if from.SecurityContext != nil {
		c.SecurityContext = from.SecurityContext
	}

	if from.ExtraContainers != nil {
		c.ExtraContainers = from.ExtraContainers
	}
	if from.InitContainers != nil {
		c.InitContainers = from.InitContainers
	}
	if from.NodeSelector != nil {
		c.NodeSelector = from.NodeSelector
	}
	if from.TopologySpreadConstraints != nil {
		c.TopologySpreadConstraints = from.TopologySpreadConstraints
	}
	if from.Tolerations != nil {
		c.Tolerations = from.Tolerations
	}
	if from.Annotations != nil {
		if c.Annotations == nil {
			c.Annotations = make(map[string]string)
		}
		maps.Copy(c.Annotations, from.Annotations)
	}
	if from.Labels != nil {
		if c.Labels == nil {
			c.Labels = make(map[string]string)
		}
		maps.Copy(c.Labels, from.Labels)
	}
	if from.TerminationGracePeriodSeconds != 0 {
		c.TerminationGracePeriodSeconds = from.TerminationGracePeriodSeconds
	}

	// Handle affinity
	if from.Affinity.NodeAffinity != nil || from.Affinity.PodAffinity != nil || from.Affinity.PodAntiAffinity != nil {
		c.Affinity = from.Affinity
	}
}
