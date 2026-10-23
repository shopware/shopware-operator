package secret

import (
	"context"
	"slices"
	"testing"

	v1 "github.com/shopware/shopware-operator/api/v1"
	"github.com/shopware/shopware-operator/internal/cronjob"
	"github.com/shopware/shopware-operator/internal/deployment"
	"github.com/shopware/shopware-operator/internal/job"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/client/interceptor"
)

func storeWithAllSecretRefs() *v1.Store {
	return &v1.Store{
		ObjectMeta: metav1.ObjectMeta{Name: "full-store", Namespace: "default"},
		Spec: v1.StoreSpec{
			SecretName: "store-secret",
			Database: v1.DatabaseSpec{
				HostRef:           v1.SecretRef{Name: "db-host", Key: "host"},
				Port:              3306,
				Version:           "8.0",
				User:              "shopware",
				Name:              "shopware",
				PasswordSecretRef: v1.SecretRef{Name: "db-password", Key: "password"},
				TLS: v1.DatabaseTLS{
					SecretName:        "db-tls",
					ClientCertificate: true,
				},
			},
			OpensearchSpec: v1.OpensearchSpec{
				Enabled:           true,
				Host:              "opensearch",
				Username:          "admin",
				PasswordSecretRef: v1.SecretRef{Name: "opensearch", Key: "password"},
				Port:              9200,
				Schema:            "https",
			},
			ShopConfiguration: v1.Configuration{
				Fastly: v1.FastlySpec{
					TokenRef:   v1.SecretRef{Name: "fastly-token", Key: "token"},
					ServiceRef: v1.SecretRef{Name: "fastly-service", Key: "service"},
				},
			},
			AdminCredentials: v1.Credentials{
				UsernameSecretRef: v1.SecretRef{Name: "admin-username", Key: "username"},
				PasswordSecretRef: v1.SecretRef{Name: "admin-password", Key: "password"},
			},
			Tideways: v1.TidewaysSpec{
				Enabled:   true,
				Service:   "shop",
				APIKeyRef: v1.SecretRef{Name: "tideways", Key: "api-key"},
			},
			S3Storage: v1.S3Storage{
				EndpointURL:        "https://s3.example.com",
				PrivateBucketName:  "private",
				PublicBucketName:   "public",
				AccessKeyRef:       v1.SecretRef{Name: "s3-access", Key: "access"},
				SecretAccessKeyRef: v1.SecretRef{Name: "s3-secret", Key: "secret"},
			},
			Network: v1.NetworkSpec{
				TLSSecretName: "store-tls",
			},
		},
	}
}

func referencedInputSecrets(namespace string) []client.Object {
	secrets := map[string]map[string][]byte{
		"db-host":        {"host": []byte("db.example.com")},
		"db-password":    {"password": []byte("db-password")},
		"db-tls":         {"ca.crt": []byte("ca"), "tls.crt": []byte("crt"), "tls.key": []byte("key")},
		"opensearch":     {"password": []byte("opensearch-password")},
		"fastly-token":   {"token": []byte("fastly-token")},
		"fastly-service": {"service": []byte("fastly-service")},
		"admin-username": {"username": []byte("admin")},
		"admin-password": {"password": []byte("admin-password")},
		"tideways":       {"api-key": []byte("tideways-key")},
		"s3-access":      {"access": []byte("access")},
		"s3-secret":      {"secret": []byte("secret")},
	}

	objs := make([]client.Object, 0, len(secrets))
	for name, data := range secrets {
		objs = append(objs, &corev1.Secret{
			ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: namespace},
			Data:       data,
		})
	}
	return objs
}

func podSpecSecretNames(spec corev1.PodSpec) []string {
	var names []string
	for _, container := range slices.Concat(spec.InitContainers, spec.Containers) {
		for _, env := range container.Env {
			if env.ValueFrom != nil && env.ValueFrom.SecretKeyRef != nil {
				names = append(names, env.ValueFrom.SecretKeyRef.Name)
			}
		}
		for _, envFrom := range container.EnvFrom {
			if envFrom.SecretRef != nil {
				names = append(names, envFrom.SecretRef.Name)
			}
		}
	}
	for _, volume := range spec.Volumes {
		if volume.Secret != nil {
			names = append(names, volume.Secret.SecretName)
		}
		if volume.Projected != nil {
			for _, source := range volume.Projected.Sources {
				if source.Secret != nil {
					names = append(names, source.Secret.Name)
				}
			}
		}
	}
	return names
}

func TestReferencedSecretNamesCoverGeneratedResources(t *testing.T) {
	ctx := context.Background()
	store := storeWithAllSecretRefs()

	scheme := runtime.NewScheme()
	require.NoError(t, v1.AddToScheme(scheme))
	require.NoError(t, corev1.AddToScheme(scheme))

	var used []string
	fakeClient := fake.NewClientBuilder().
		WithScheme(scheme).
		WithObjects(referencedInputSecrets(store.Namespace)...).
		WithInterceptorFuncs(interceptor.Funcs{
			Get: func(ctx context.Context, c client.WithWatch, key client.ObjectKey, obj client.Object, opts ...client.GetOption) error {
				if _, ok := obj.(*corev1.Secret); ok {
					used = append(used, key.Name)
				}
				return c.Get(ctx, key, obj, opts...)
			},
		}).
		Build()

	_, err := EnsureStoreSecret(ctx, fakeClient, &mockEventRecorder{}, store)
	require.NoError(t, err)

	workers, err := deployment.WorkerDeployments(*store)
	require.NoError(t, err)

	podSpecs := make([]corev1.PodSpec, 0, 5+len(workers))
	podSpecs = append(podSpecs,
		deployment.StorefrontDeployment(*store).Spec.Template.Spec,
		deployment.AdminDeployment(*store).Spec.Template.Spec,
		job.SetupJob(*store).Spec.Template.Spec,
		job.MigrationJob(*store).Spec.Template.Spec,
		cronjob.ScheduledTaskJob(*store).Spec.JobTemplate.Spec.Template.Spec,
	)
	for _, worker := range workers {
		podSpecs = append(podSpecs, worker.Spec.Template.Spec)
	}
	for _, spec := range podSpecs {
		used = append(used, podSpecSecretNames(spec)...)
	}

	used = slices.DeleteFunc(used, func(name string) bool {
		return name == store.GetSecretName()
	})
	slices.Sort(used)
	used = slices.Compact(used)

	require.ElementsMatch(t, used, referencedSecretNames(store))
}

func TestEnsureStoreSecretIsStable(t *testing.T) {
	ctx := context.Background()
	store := storeWithAllSecretRefs()

	scheme := runtime.NewScheme()
	require.NoError(t, v1.AddToScheme(scheme))
	require.NoError(t, corev1.AddToScheme(scheme))

	fakeClient := fake.NewClientBuilder().
		WithScheme(scheme).
		WithObjects(referencedInputSecrets(store.Namespace)...).
		Build()

	first, err := EnsureStoreSecret(ctx, fakeClient, &mockEventRecorder{}, store)
	require.NoError(t, err)
	require.NoError(t, fakeClient.Create(ctx, first))

	second, err := EnsureStoreSecret(ctx, fakeClient, &mockEventRecorder{}, store)
	require.NoError(t, err)

	for key, value := range first.Data {
		require.Containsf(t, second.Data, key, "key %q disappeared after regenerating the store secret", key)
		require.Equalf(t, string(value), string(second.Data[key]),
			"key %q changed after regenerating the store secret, generated values must only be set when missing", key)
	}
	require.Len(t, second.Data, len(first.Data))
}
