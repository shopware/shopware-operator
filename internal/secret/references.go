package secret

import (
	"slices"

	v1 "github.com/shopware/shopware-operator/api/v1"
)

func ReferencedSecretNames(store *v1.Store) []string {
	candidates := []string{
		store.GetSecretName(),
		store.Spec.Database.PasswordSecretRef.Name,
		store.Spec.Database.HostRef.Name,
		store.Spec.Database.TLS.SecretName,
		store.Spec.OpensearchSpec.PasswordSecretRef.Name,
		store.Spec.ShopConfiguration.Fastly.TokenRef.Name,
		store.Spec.ShopConfiguration.Fastly.ServiceRef.Name,
		store.Spec.AdminCredentials.UsernameSecretRef.Name,
		store.Spec.AdminCredentials.PasswordSecretRef.Name,
		store.Spec.S3Storage.AccessKeyRef.Name,
		store.Spec.S3Storage.SecretAccessKeyRef.Name,
		store.Spec.Tideways.APIKeyRef.Name,
	}

	names := make([]string, 0, len(candidates))
	for _, name := range candidates {
		if name != "" && !slices.Contains(names, name) {
			names = append(names, name)
		}
	}
	return names
}

func ReferencesSecret(store *v1.Store, name string) bool {
	return slices.Contains(ReferencedSecretNames(store), name)
}
