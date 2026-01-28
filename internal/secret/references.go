package secret

import (
	"slices"

	v1 "github.com/shopware/shopware-operator/api/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

func referencedSecretNames(store *v1.Store) []string {
	spec := store.Spec
	candidates := []string{
		spec.Database.PasswordSecretRef.Name,
		spec.Database.HostRef.Name,
		spec.Database.TLS.SecretName,
		spec.OpensearchSpec.PasswordSecretRef.Name,
		spec.ShopConfiguration.Fastly.TokenRef.Name,
		spec.ShopConfiguration.Fastly.ServiceRef.Name,
		spec.AdminCredentials.UsernameSecretRef.Name,
		spec.AdminCredentials.PasswordSecretRef.Name,
		spec.Tideways.APIKeyRef.Name,
		spec.S3Storage.AccessKeyRef.Name,
		spec.S3Storage.SecretAccessKeyRef.Name,
	}

	names := make([]string, 0, len(candidates))
	for _, name := range candidates {
		if name != "" && !slices.Contains(names, name) {
			names = append(names, name)
		}
	}
	return names
}

func IsReferencedByStore(store *v1.Store, secret client.Object) bool {
	return store.Namespace == secret.GetNamespace() &&
		slices.Contains(referencedSecretNames(store), secret.GetName())
}
