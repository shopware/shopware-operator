package v1

import (
	"fmt"
	"strings"

	corev1 "k8s.io/api/core/v1"
)

const (
	WorkerAdapterBuiltin = "builtin"
	WorkerAdapterRedis   = "redis"
	WorkerAdapterSQS     = "sqs"

	messengerTransportDSNEnv     = "MESSENGER_TRANSPORT_DSN"
	messengerSQSAccessKeyIDEnv   = "MESSENGER_SQS_ACCESS_KEY_ID"
	messengerSQSSecretKeyEnv     = "MESSENGER_SQS_SECRET_ACCESS_KEY"
	defaultSQSEndpoint           = "default"
	defaultSQSQueueName          = "messages"
	defaultRedisMessengerDSNTmpl = "redis://%s:%d/messages/symfony?auto_setup=true&serializer=1&stream_max_entries=0&dbindex=%d"
)

// +kubebuilder:object:generate=false
type QueueTransport interface {
	EnvVars() []corev1.EnvVar
}

func (w WorkerSpec) Transport() QueueTransport {
	switch w.Adapter {
	case WorkerAdapterRedis:
		return RedisQueue{RedisSpec: w.RedisSpec}
	case WorkerAdapterSQS:
		return w.SQS
	default:
		return nil
	}
}

type RedisQueue struct {
	RedisSpec
}

func (r RedisQueue) DSN() string {
	if r.RedisDSN != "" {
		return r.RedisDSN
	}
	return fmt.Sprintf(defaultRedisMessengerDSNTmpl, r.RedisHost, r.RedisPort, r.RedisIndex)
}

func (r RedisQueue) EnvVars() []corev1.EnvVar {
	return []corev1.EnvVar{
		{
			Name:  messengerTransportDSNEnv,
			Value: r.DSN(),
		},
		{
			Name: "MESSENGER_CONSUMER_NAME",
			ValueFrom: &corev1.EnvVarSource{
				FieldRef: &corev1.ObjectFieldSelector{
					APIVersion: "v1",
					FieldPath:  "metadata.name",
				},
			},
		},
	}
}

type SQSSpec struct {
	// +kubebuilder:description="Full symfony messenger SQS DSN. Takes precedence over all other fields."
	DSN string `json:"dsn,omitempty"`
	// +kubebuilder:description="Secret key holding the full symfony messenger SQS DSN. Takes precedence over all fields except dsn."
	DSNSecretRef SecretRef `json:"dsnSecretRef,omitempty"`

	// +kubebuilder:description="SQS endpoint as host[:port]. Leave empty to use the AWS endpoint of the region. Set it for any SQS compatible service."
	Endpoint string `json:"endpoint,omitempty"`
	Region   string `json:"region,omitempty"`
	Account  string `json:"account,omitempty"`
	// +kubebuilder:default=messages
	QueueName string `json:"queueName,omitempty"`
	// +kubebuilder:example=sslmode=disable&auto_setup=false
	Options string `json:"options,omitempty"`

	// +kubebuilder:description="Secret key holding the access key id. The value must be URL encoded."
	AccessKeyRef SecretRef `json:"accessKeyRef,omitempty"`
	// +kubebuilder:description="Secret key holding the secret access key. The value must be URL encoded."
	SecretAccessKeyRef SecretRef `json:"secretAccessKeyRef,omitempty"`
}

func (s SQSSpec) DSNValue() string {
	if s.DSN != "" {
		return s.DSN
	}

	endpoint := s.Endpoint
	if endpoint == "" {
		endpoint = defaultSQSEndpoint
	}

	queueName := s.QueueName
	if queueName == "" {
		queueName = defaultSQSQueueName
	}

	path := "/" + queueName
	if s.Account != "" {
		path = "/" + s.Account + path
	}

	userInfo := ""
	if s.AccessKeyRef.IsSet() && s.SecretAccessKeyRef.IsSet() {
		userInfo = fmt.Sprintf("$(%s):$(%s)@", messengerSQSAccessKeyIDEnv, messengerSQSSecretKeyEnv)
	}

	query := make([]string, 0, 2)
	if s.Region != "" {
		query = append(query, "region="+s.Region)
	}
	if options := strings.TrimPrefix(s.Options, "?"); options != "" {
		query = append(query, options)
	}

	dsn := "sqs://" + userInfo + endpoint + path
	if len(query) > 0 {
		dsn += "?" + strings.Join(query, "&")
	}
	return dsn
}

func (s SQSSpec) EnvVars() []corev1.EnvVar {
	if s.DSN == "" && s.DSNSecretRef.IsSet() {
		return []corev1.EnvVar{
			{
				Name:      messengerTransportDSNEnv,
				ValueFrom: secretKeyRefSource(s.DSNSecretRef),
			},
		}
	}

	envVars := make([]corev1.EnvVar, 0, 3)
	if s.DSN == "" && s.AccessKeyRef.IsSet() && s.SecretAccessKeyRef.IsSet() {
		envVars = append(envVars,
			corev1.EnvVar{
				Name:      messengerSQSAccessKeyIDEnv,
				ValueFrom: secretKeyRefSource(s.AccessKeyRef),
			},
			corev1.EnvVar{
				Name:      messengerSQSSecretKeyEnv,
				ValueFrom: secretKeyRefSource(s.SecretAccessKeyRef),
			},
		)
	}

	return append(envVars, corev1.EnvVar{
		Name:  messengerTransportDSNEnv,
		Value: s.DSNValue(),
	})
}

func secretKeyRefSource(ref SecretRef) *corev1.EnvVarSource {
	return &corev1.EnvVarSource{
		SecretKeyRef: &corev1.SecretKeySelector{
			LocalObjectReference: corev1.LocalObjectReference{
				Name: ref.Name,
			},
			Key: ref.Key,
		},
	}
}
