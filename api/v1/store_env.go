package v1

import (
	"fmt"
	"slices"
	"strconv"

	corev1 "k8s.io/api/core/v1"
)

// TODO: If building more than one instance print a warning for the cache to use
// redis
func (s *Store) getAppCache() []corev1.EnvVar {
	if s.Spec.AppCache.Adapter == "redis" {
		// If DSN is provided, use it directly
		if s.Spec.AppCache.RedisDSN != "" {
			return []corev1.EnvVar{
				{
					Name:  "K8S_CACHE_TYPE",
					Value: "redis",
				},
				{
					Name:  "K8S_REDIS_APP_DSN",
					Value: s.Spec.AppCache.RedisDSN,
				},
			}
		}

		// Otherwise build from individual fields
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
// DEPRECATED: (6.7) Use symfony cache instead
func (s *Store) getOldSessionCache() []corev1.EnvVar {
	if s.Spec.SessionCache.Adapter == "redis" {
		var savePath string
		// If DSN is provided, use it directly
		if s.Spec.SessionCache.RedisDSN != "" {
			return []corev1.EnvVar{
				{
					Name:  "REDIS_SESSION_DSN",
					Value: s.Spec.SessionCache.RedisDSN,
				},
				{
					Name:  "PHP_SESSION_HANDLER",
					Value: "redis",
				},
			}
		} else {
			// Otherwise build from individual fields
			savePath = fmt.Sprintf(
				"tcp://%s:%d/%d",
				s.Spec.SessionCache.RedisHost,
				s.Spec.SessionCache.RedisPort,
				s.Spec.SessionCache.RedisIndex,
			)
		}

		return []corev1.EnvVar{
			{
				Name:  "PHP_SESSION_HANDLER",
				Value: "redis",
			},
			{
				Name:  "PHP_SESSION_SAVE_PATH",
				Value: savePath,
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

// Added in 6.7
func (s *Store) getSessionCache() []corev1.EnvVar {
	if s.Spec.SessionCache.Adapter == "redis" {
		var dsn string
		// If DSN is provided, use it directly
		if s.Spec.SessionCache.RedisDSN != "" {
			dsn = s.Spec.SessionCache.RedisDSN
		} else {
			// Otherwise build from individual fields
			dsn = fmt.Sprintf(
				"redis://%s:%d/%d",
				s.Spec.SessionCache.RedisHost,
				s.Spec.SessionCache.RedisPort,
				s.Spec.SessionCache.RedisIndex,
			)
		}

		return []corev1.EnvVar{
			{
				Name:  "K8S_REDIS_SESSION_DSN",
				Value: dsn,
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
		// If DSN is provided, use it directly
		if s.Spec.Worker.RedisDSN != "" {
			return []corev1.EnvVar{
				{
					Name:  "MESSENGER_TRANSPORT_DSN",
					Value: s.Spec.Worker.RedisDSN,
				},
			}
		}

		// Otherwise build from individual fields
		return []corev1.EnvVar{
			{
				Name: "MESSENGER_TRANSPORT_DSN",
				Value: fmt.Sprintf(
					"redis://%s:%d/messages/symfony/consumer?auto_setup=true&serializer=1&stream_max_entries=0&dbindex=%d",
					s.Spec.Worker.RedisHost,
					s.Spec.Worker.RedisPort,
					s.Spec.Worker.RedisIndex,
				),
			},
		}
	}
	return []corev1.EnvVar{}
}

func (s *Store) getOpensearch() []corev1.EnvVar {
	if s.Spec.OpensearchSpec.Enabled {
		return []corev1.EnvVar{
			{
				Name: "OPENSEARCH_URL",
				ValueFrom: &corev1.EnvVarSource{
					SecretKeyRef: &corev1.SecretKeySelector{
						LocalObjectReference: corev1.LocalObjectReference{
							Name: s.Spec.SecretName,
						},
						Key: "opensearch-url",
					},
				},
			},
			{
				Name: "ADMIN_OPENSEARCH_URL",
				ValueFrom: &corev1.EnvVarSource{
					SecretKeyRef: &corev1.SecretKeySelector{
						LocalObjectReference: corev1.LocalObjectReference{
							Name: s.Spec.SecretName,
						},
						Key: "opensearch-url",
					},
				},
			},
			{
				Name:  "SHOPWARE_ES_ENABLED",
				Value: "1",
			},
			{
				Name:  "SHOPWARE_ES_INDEXING_ENABLED",
				Value: "1",
			},
			{
				Name:  "SHOPWARE_ADMIN_ES_ENABLED",
				Value: "1",
			},
			{
				Name:  "SHOPWARE_ADMIN_ES_REFRESH_INDICES",
				Value: "1",
			},
			{
				Name:  "SHOPWARE_ES_THROW_EXCEPTION",
				Value: "1",
			},
			{
				Name:  "K8S_ES_NUMBER_OF_REPLICAS",
				Value: strconv.Itoa(s.Spec.OpensearchSpec.Index.Replicas),
			},
			{
				Name:  "K8S_ES_NUMBER_OF_SHARDS",
				Value: strconv.Itoa(s.Spec.OpensearchSpec.Index.Shards),
			},
			{
				Name:  "SHOPWARE_ES_INDEX_PREFIX",
				Value: s.Spec.OpensearchSpec.Index.Prefix,
			},
			{
				Name:  "SHOPWARE_ADMIN_ES_INDEX_PREFIX",
				Value: fmt.Sprintf("%s-admin", s.Spec.OpensearchSpec.Index.Prefix),
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
	envVars := []corev1.EnvVar{
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
	}

	if s.Spec.S3Storage.AccessKeyRef.Name != "" {
		envVars = append(envVars, corev1.EnvVar{
			Name: "AWS_ACCESS_KEY_ID",
			ValueFrom: &corev1.EnvVarSource{
				SecretKeyRef: &corev1.SecretKeySelector{
					LocalObjectReference: corev1.LocalObjectReference{
						Name: s.Spec.S3Storage.AccessKeyRef.Name,
					},
					Key: s.Spec.S3Storage.AccessKeyRef.Key,
				},
			},
		})
	}

	if s.Spec.S3Storage.SecretAccessKeyRef.Key != "" {
		envVars = append(envVars, corev1.EnvVar{
			Name: "AWS_SECRET_ACCESS_KEY",
			ValueFrom: &corev1.EnvVarSource{
				SecretKeyRef: &corev1.SecretKeySelector{
					LocalObjectReference: corev1.LocalObjectReference{
						Name: s.Spec.S3Storage.SecretAccessKeyRef.Name,
					},
					Key: s.Spec.S3Storage.SecretAccessKeyRef.Key,
				},
			},
		})
	}

	return envVars
}

func (s *Store) getFastly() []corev1.EnvVar {
	envVars := []corev1.EnvVar{}
	if s.Spec.ShopConfiguration.Fastly.ServiceRef.Name != "" &&
		s.Spec.ShopConfiguration.Fastly.ServiceRef.Key != "" &&
		s.Spec.ShopConfiguration.Fastly.TokenRef.Name != "" &&
		s.Spec.ShopConfiguration.Fastly.TokenRef.Key != "" {
		envVars = append(envVars,
			corev1.EnvVar{
				Name: "FASTLY_SERVICE_ID",
				ValueFrom: &corev1.EnvVarSource{
					SecretKeyRef: &corev1.SecretKeySelector{
						LocalObjectReference: corev1.LocalObjectReference{
							Name: s.Spec.SecretName,
						},
						Key: "fastly-service-id",
					},
				},
			},
			corev1.EnvVar{
				Name: "FASTLY_API_TOKEN",
				ValueFrom: &corev1.EnvVarSource{
					SecretKeyRef: &corev1.SecretKeySelector{
						LocalObjectReference: corev1.LocalObjectReference{
							Name: s.Spec.SecretName,
						},
						Key: "fastly-token",
					},
				},
			},
		)
	}
	return envVars
}

func (s *Store) GetEnv() []corev1.EnvVar {
	var appUrl string
	if s.Spec.Network.AppURLHost == "" {
		appUrl = fmt.Sprintf("https://%s", s.Spec.Network.Host)
	} else {
		appUrl = fmt.Sprintf("https://%s", s.Spec.Network.AppURLHost)
	}

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
		// Changed in 6.7
		{
			Name:  "SYMFONY_TRUSTED_PROXIES",
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
			Name:  "INSTALL_LOCALE",
			Value: s.Spec.ShopConfiguration.Locale,
		},
		{
			Name:  "INSTALL_CURRENCY",
			Value: s.Spec.ShopConfiguration.Currency,
		},
		{
			Name:  "APP_URL",
			Value: appUrl,
		},
		{
			Name:  "DATABASE_PERSISTENT_CONNECTION",
			Value: "1",
		},
	}

	if s.Spec.ShopConfiguration.UsageDataConsent == "revoked" {
		c = append(c, corev1.EnvVar{
			Name:  "SHOPWARE_USAGE_DATA_CONSENT",
			Value: "revoked",
		})
	}

	c = append(c, s.getOldSessionCache()...)
	c = append(c, s.getSessionCache()...)
	c = append(c, s.getAppCache()...)
	c = append(c, s.getOtel()...)
	c = append(c, s.getBlackfire()...)
	c = append(c, s.getStorage()...)
	c = append(c, s.getWorker()...)
	c = append(c, s.getOpensearch()...)
	c = append(c, s.getFastly()...)
	c = append(c, s.Spec.FPM.getFPMConfiguration()...)

	for _, obj2 := range s.Spec.Container.ExtraEnvs {
		if i := slices.IndexFunc(c, func(c corev1.EnvVar) bool { return c.Name == obj2.Name }); i > -1 {
			c[i] = obj2
		} else {
			c = append(c, obj2)
		}
	}

	return c
}
