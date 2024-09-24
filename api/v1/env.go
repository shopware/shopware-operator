package v1

import (
	"fmt"
	"strconv"

	corev1 "k8s.io/api/core/v1"
)

// TODO: If building more than one instance print a warning for the cache to use
// redis
func (s *Store) getAppCache() []corev1.EnvVar {
	if s.Spec.AppCache.Adapter == "redis" {
		return []corev1.EnvVar{
			// TODO: Will be moved to yaml configuration
			{
				Name:  "K8S_CACHE_TYPE",
				Value: "redis",
			},
			{
				Name:  "K8S_CACHE_HOST",
				Value: s.Spec.AppCache.RedisHost,
			},
			{
				Name:  "K8S_CACHE_PORT",
				Value: fmt.Sprintf("%d", s.Spec.AppCache.RedisPort),
			},
			{
				Name:  "K8S_CACHE_INDEX",
				Value: fmt.Sprintf("%d", s.Spec.AppCache.RedisIndex),
			},
			// DEPRECATED: This is deprecated see: https://github.com/shopware/recipes/pull/112
			{
				Name: "K8S_CACHE_URL",
				Value: fmt.Sprintf(
					"redis://%s:%d/%d",
					s.Spec.AppCache.RedisHost,
					s.Spec.AppCache.RedisPort,
					s.Spec.AppCache.RedisIndex,
				),
			},
		}
	}
	return []corev1.EnvVar{}
}

// Handled by PHP itself
func (s *Store) getSessionCache() []corev1.EnvVar {
	if s.Spec.SessionCache.Adapter == "redis" {
		return []corev1.EnvVar{
			{
				Name:  "PHP_SESSION_HANDLER",
				Value: "redis",
			},
			{
				Name: "PHP_SESSION_SAVE_PATH",
				Value: fmt.Sprintf(
					"tcp://%s:%d/%d",
					s.Spec.SessionCache.RedisHost,
					s.Spec.SessionCache.RedisPort,
					s.Spec.SessionCache.RedisIndex,
				),
			},
		}
	}
	return []corev1.EnvVar{
		{
			Name:  "PHP_SESSION_HANDLER",
			Value: "files",
		},
	}
}

func (f *FPMSpec) getFPMConfiguration() []corev1.EnvVar {
	if f.ProcessManagement != "dynamic" {
		return []corev1.EnvVar{
			{
				Name:  "FPM_PM",
				Value: f.ProcessManagement,
			},
			{
				// This should be always socket and not tcp
				Name:  "FPM_LISTEN",
				Value: f.Listen,
			},
			{
				Name:  "PHP_FPM_SCRAPE_URI",
				Value: f.ScrapeURI,
			},
			{
				Name:  "FPM_PM_STATUS_PATH",
				Value: f.StatusPath,
			},
			{
				Name:  "FPM_PM_MAX_CHILDREN",
				Value: strconv.Itoa(f.MaxChildren),
			},
			{
				Name:  "FPM_PM_START_SERVERS",
				Value: strconv.Itoa(f.StartServers),
			},
			{
				Name:  "FPM_PM_MIN_SPARE_SERVERS",
				Value: strconv.Itoa(f.MinSpareServers),
			},
			{
				Name:  "FPM_PM_MAX_SPARE_SERVERS",
				Value: strconv.Itoa(f.MaxSpareServers),
			},
		}
	}
	return []corev1.EnvVar{
		{
			Name:  "FPM_PM",
			Value: f.ProcessManagement,
		},
	}
}

// https://symfony.com/doc/current/messenger.html#transport-configuration
func (s *Store) getWorker() []corev1.EnvVar {
	if s.Spec.Worker.Adapter == "redis" {
		return []corev1.EnvVar{
			{
				Name: "MESSENGER_TRANSPORT_DSN",
				Value: fmt.Sprintf(
					"rediss://%s:%d/messages/symfony/consumer?auto_setup=true&serializer=1&stream_max_entries=0&dbindex=%d",
					s.Spec.AppCache.RedisHost,
					s.Spec.AppCache.RedisPort,
					s.Spec.AppCache.RedisIndex,
				),
			},
		}
	}
	return []corev1.EnvVar{}
}

// TODO: blackfire and otel together not working
func (s *Store) getOtel() []corev1.EnvVar {
	if s.Spec.Otel.Enabled {
		return []corev1.EnvVar{
			{
				Name:  "OTEL_PHP_AUTOLOAD_ENABLED",
				Value: "true",
			},
			{
				Name:  "OTEL_SERVICE_NAME",
				Value: s.Spec.Otel.ServiceName,
			},
			{
				Name:  "OTEL_TRACES_EXPORTER",
				Value: s.Spec.Otel.TracesExporter,
			},
			{
				Name:  "OTEL_EXPORTER_OTLP_PROTOCOL",
				Value: s.Spec.Otel.ExporterProtocol,
			},
			{
				Name:  "OTEL_EXPORTER_OTLP_ENDPOINT",
				Value: s.Spec.Otel.ExporterEndpoint,
			},
		}
	}
	return []corev1.EnvVar{}
}

// TODO: blackfire and otel together not working
func (s *Store) getBlackfire() []corev1.EnvVar {
	if s.Spec.Blackfire.Enabled {
		return []corev1.EnvVar{
			{
				Name: "BLACKFIRE_AGENT_SOCKET",
				Value: fmt.Sprintf(
					"tcp://%s:%d",
					s.Spec.Blackfire.Host,
					s.Spec.Blackfire.Port,
				),
			},
		}
	}
	return []corev1.EnvVar{}
}

// TODO: Minimum s3 storage no filesystem support
// TODO: Minio should use bucketname before URL. So we have public.domain.com see:
// https://min.io/docs/minio/linux/administration/object-management.html#minio-object-management-path-virtual-access
func (s *Store) getStorage() []corev1.EnvVar {
	return []corev1.EnvVar{
		{
			Name:  "K8S_FILESYSTEM_PUBLIC_BUCKET",
			Value: s.Spec.S3Storage.PublicBucketName,
		},
		{
			Name:  "K8S_FILESYSTEM_PRIVATE_BUCKET",
			Value: s.Spec.S3Storage.PrivateBucketName,
		},
		{
			Name:  "K8S_FILESYSTEM_PUBLIC_URL",
			Value: s.Spec.CDNURL,
		},
		{
			// Region should be set
			Name:  "K8S_FILESYSTEM_REGION",
			Value: s.Spec.S3Storage.Region,
		},
		{
			Name:  "K8S_FILESYSTEM_ENDPOINT",
			Value: s.Spec.S3Storage.EndpointURL,
		},
		{
			Name: "AWS_ACCESS_KEY_ID",
			ValueFrom: &corev1.EnvVarSource{
				SecretKeyRef: &corev1.SecretKeySelector{
					LocalObjectReference: corev1.LocalObjectReference{
						Name: s.Spec.S3Storage.AccessKeyRef.Name,
					},
					Key: s.Spec.S3Storage.AccessKeyRef.Key,
				},
			},
		},
		{
			Name: "AWS_SECRET_ACCESS_KEY",
			ValueFrom: &corev1.EnvVarSource{
				SecretKeyRef: &corev1.SecretKeySelector{
					LocalObjectReference: corev1.LocalObjectReference{
						Name: s.Spec.S3Storage.SecretAccessKeyRef.Name,
					},
					Key: s.Spec.S3Storage.SecretAccessKeyRef.Key,
				},
			},
		},
	}
}

func (s *Store) GetEnv() []corev1.EnvVar {

	// TODO: Use map for overwriting when using ENVs from customer
	c := []corev1.EnvVar{
		{
			Name: "DATABASE_URL",
			ValueFrom: &corev1.EnvVarSource{
				SecretKeyRef: &corev1.SecretKeySelector{
					LocalObjectReference: corev1.LocalObjectReference{
						Name: s.Spec.SecretName,
					},
					Key: "database-url",
				},
			},
		},
		{
			Name: "APP_SECRET",
			ValueFrom: &corev1.EnvVarSource{
				SecretKeyRef: &corev1.SecretKeySelector{
					LocalObjectReference: corev1.LocalObjectReference{
						Name: s.Spec.SecretName,
					},
					Key: "app-secret",
				},
			},
		},
		{
			Name: "JWT_PRIVATE_KEY",
			ValueFrom: &corev1.EnvVarSource{
				SecretKeyRef: &corev1.SecretKeySelector{
					LocalObjectReference: corev1.LocalObjectReference{
						Name: s.Spec.SecretName,
					},
					Key: "jwt-private-key",
				},
			},
		},
		{
			Name: "JWT_PUBLIC_KEY",
			ValueFrom: &corev1.EnvVarSource{
				SecretKeyRef: &corev1.SecretKeySelector{
					LocalObjectReference: corev1.LocalObjectReference{
						Name: s.Spec.SecretName,
					},
					Key: "jwt-public-key",
				},
			},
		},
		// When using https we need this for header trusting
		{
			Name:  "TRUSTED_PROXIES",
			Value: "REMOTE_ADDR",
		},
		// Some Shopware best practises
		{
			Name:  "SHOPWARE_DBAL_TIMEZONE_SUPPORT_ENABLED",
			Value: "1",
		},
		{
			Name:  "SQL_SET_DEFAULT_SESSION_VARIABLES",
			Value: "0",
		},
		{
			Name:  "APP_URL",
			Value: fmt.Sprintf("https://%s", s.Spec.Network.Host),
		},
	}

	c = append(c, s.getSessionCache()...)
	c = append(c, s.getAppCache()...)
	c = append(c, s.getOtel()...)
	c = append(c, s.getBlackfire()...)
	c = append(c, s.getStorage()...)
	c = append(c, s.getWorker()...)
	c = append(c, s.Spec.FPM.getFPMConfiguration()...)

	return append(c, s.Spec.Container.ExtraEnvs...)
}
