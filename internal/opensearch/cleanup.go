package opensearch

import (
	"context"
	"crypto/tls"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	v1 "github.com/shopware/shopware-operator/api/v1"
	"github.com/shopware/shopware-operator/internal/logging"
	"go.uber.org/zap"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// CleanupResources deletes all Opensearch resources (indices and aliases) for the given store
func CleanupResources(ctx context.Context, k8sClient client.Client, store *v1.Store) error {
	log := logging.FromContext(ctx)

	if !store.Spec.OpensearchSpec.Enabled {
		log.Info("Opensearch is not enabled for this store, skipping cleanup")
		return nil
	}

	if !store.Spec.OpensearchSpec.CleanupOnDeletion {
		log.Info("Opensearch cleanup on deletion is not enabled for this store, skipping cleanup")
		return nil
	}

	// Get credentials from the store secret
	credentials, err := getOpensearchCredentials(ctx, k8sClient, store)
	if err != nil {
		return fmt.Errorf("failed to get opensearch credentials: %w", err)
	}

	// Create HTTP client with proper TLS configuration
	// TLS certificate verification is enabled by default for security.
	// For production environments with self-signed certificates, configure the system CA pool
	// or mount custom CA certificates into the operator pod at /etc/ssl/certs/
	httpClient := &http.Client{
		Timeout: 30 * time.Second,
		Transport: &http.Transport{
			TLSClientConfig: &tls.Config{
				InsecureSkipVerify: false, // Always verify certificates for security
			},
		},
	}

	baseURL := fmt.Sprintf("%s://%s:%d",
		store.Spec.OpensearchSpec.Schema,
		store.Spec.OpensearchSpec.Host,
		store.Spec.OpensearchSpec.Port,
	)

	prefix := store.Spec.OpensearchSpec.Index.Prefix
	log.Infow("Starting Opensearch cleanup",
		zap.String("baseURL", baseURL),
		zap.String("prefix", prefix),
	)

	// List all indices with the prefix
	indices, err := listIndices(ctx, httpClient, baseURL, credentials, prefix)
	if err != nil {
		return fmt.Errorf("failed to list indices: %w", err)
	}

	log.Infow("Found indices to delete", zap.Int("count", len(indices)))

	// Track deletion errors
	var deletionErrors []error

	// Delete regular indices
	deletionErrors = append(deletionErrors, deleteIndicesBatch(ctx, httpClient, baseURL, credentials, indices, "index")...)

	// Also delete aliases with the prefix (admin index)
	adminPrefix := fmt.Sprintf("%s-admin", prefix)
	adminIndices, err := listIndices(ctx, httpClient, baseURL, credentials, adminPrefix)
	if err != nil {
		log.Warnw("Failed to list admin indices", zap.Error(err))
	} else {
		log.Infow("Found admin indices to delete", zap.Int("count", len(adminIndices)))
		deletionErrors = append(deletionErrors, deleteIndicesBatch(ctx, httpClient, baseURL, credentials, adminIndices, "admin index")...)
	}

	// Return error if any deletions failed
	if len(deletionErrors) > 0 {
		return fmt.Errorf("failed to delete %d indices: %w", len(deletionErrors), errors.Join(deletionErrors...))
	}

	log.Info("Opensearch cleanup completed")
	return nil
}

// deleteIndicesBatch deletes a batch of indices and returns any errors encountered
func deleteIndicesBatch(ctx context.Context, httpClient *http.Client, baseURL string, credentials *opensearchCredentials, indices []string, indexType string) []error {
	log := logging.FromContext(ctx)
	var deletionErrors []error

	for _, index := range indices {
		if err := deleteIndex(ctx, httpClient, baseURL, credentials, index); err != nil {
			log.Errorw("Failed to delete "+indexType, zap.String("index", index), zap.Error(err))
			deletionErrors = append(deletionErrors, fmt.Errorf("failed to delete %s %s: %w", indexType, index, err))
		} else {
			log.Infow("Successfully deleted "+indexType, zap.String("index", index))
		}
	}

	return deletionErrors
}

type opensearchCredentials struct {
	Username string
	Password string
}

func getOpensearchCredentials(ctx context.Context, k8sClient client.Client, store *v1.Store) (*opensearchCredentials, error) {
	// Get the password from the referenced secret
	secret := &corev1.Secret{}
	secretName := types.NamespacedName{
		Namespace: store.Namespace,
		Name:      store.Spec.OpensearchSpec.PasswordSecretRef.Name,
	}

	if err := k8sClient.Get(ctx, secretName, secret); err != nil {
		return nil, fmt.Errorf("failed to get opensearch password secret: %w", err)
	}

	password, ok := secret.Data[store.Spec.OpensearchSpec.PasswordSecretRef.Key]
	if !ok {
		return nil, fmt.Errorf("password key %s not found in secret %s",
			store.Spec.OpensearchSpec.PasswordSecretRef.Key,
			store.Spec.OpensearchSpec.PasswordSecretRef.Name)
	}

	return &opensearchCredentials{
		Username: store.Spec.OpensearchSpec.Username,
		Password: string(password),
	}, nil
}

func listIndices(ctx context.Context, httpClient *http.Client, baseURL string, credentials *opensearchCredentials, prefix string) ([]string, error) {
	log := logging.FromContext(ctx)

	// Use _cat/indices API to list indices
	url := fmt.Sprintf("%s/_cat/indices/%s*?format=json", baseURL, prefix)
	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	req.SetBasicAuth(credentials.Username, credentials.Password)
	req.Header.Set("Content-Type", "application/json")

	resp, err := httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to execute request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusNotFound {
		log.Info("No indices found with the given prefix")
		return []string{}, nil
	}

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("unexpected status code %d: %s", resp.StatusCode, string(body))
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response: %w", err)
	}

	var indices []map[string]interface{}
	if err := json.Unmarshal(body, &indices); err != nil {
		return nil, fmt.Errorf("failed to parse response: %w", err)
	}

	var indexNames []string
	for _, index := range indices {
		if name, ok := index["index"].(string); ok {
			// Filter out system indices (starting with .)
			if !strings.HasPrefix(name, ".") {
				indexNames = append(indexNames, name)
			}
		}
	}

	return indexNames, nil
}

func deleteIndex(ctx context.Context, httpClient *http.Client, baseURL string, credentials *opensearchCredentials, indexName string) error {
	url := fmt.Sprintf("%s/%s", baseURL, indexName)
	req, err := http.NewRequestWithContext(ctx, "DELETE", url, nil)
	if err != nil {
		return fmt.Errorf("failed to create request: %w", err)
	}

	req.SetBasicAuth(credentials.Username, credentials.Password)
	req.Header.Set("Content-Type", "application/json")

	resp, err := httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("failed to execute request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusNotFound {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("unexpected status code %d: %s", resp.StatusCode, string(body))
	}

	return nil
}
