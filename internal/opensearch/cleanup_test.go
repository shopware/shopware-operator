package opensearch

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strconv"
	"testing"

	v1 "github.com/shopware/shopware-operator/api/v1"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

func TestCleanupResources_OpensearchDisabled(t *testing.T) {
	ctx := context.Background()
	scheme := runtime.NewScheme()
	_ = corev1.AddToScheme(scheme)
	_ = v1.AddToScheme(scheme)

	store := &v1.Store{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-store",
			Namespace: "default",
		},
		Spec: v1.StoreSpec{
			OpensearchSpec: v1.OpensearchSpec{
				Enabled: false,
			},
		},
	}

	client := fake.NewClientBuilder().WithScheme(scheme).Build()

	err := CleanupResources(ctx, client, store)
	assert.NoError(t, err, "Should not error when opensearch is disabled")
}

func TestCleanupResources_CleanupDisabled(t *testing.T) {
	ctx := context.Background()
	scheme := runtime.NewScheme()
	_ = corev1.AddToScheme(scheme)
	_ = v1.AddToScheme(scheme)

	store := &v1.Store{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-store",
			Namespace: "default",
		},
		Spec: v1.StoreSpec{
			OpensearchSpec: v1.OpensearchSpec{
				Enabled:           true,
				CleanupOnDeletion: false,
			},
		},
	}

	client := fake.NewClientBuilder().WithScheme(scheme).Build()

	err := CleanupResources(ctx, client, store)
	assert.NoError(t, err, "Should not error when cleanup is disabled")
}

func TestListIndices(t *testing.T) {
	// Mock opensearch server
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "/_cat/indices/test-prefix*?format=json", r.URL.Path+"?"+r.URL.RawQuery)
		assert.Equal(t, "GET", r.Method)

		// Check basic auth
		username, password, ok := r.BasicAuth()
		assert.True(t, ok)
		assert.Equal(t, "testuser", username)
		assert.Equal(t, "testpass", password)

		indices := []map[string]interface{}{
			{"index": "test-prefix-product"},
			{"index": "test-prefix-category"},
			{"index": ".system-index"}, // Should be filtered out
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(indices)
	}))
	defer server.Close()

	ctx := context.Background()
	credentials := &opensearchCredentials{
		Username: "testuser",
		Password: "testpass",
	}

	httpClient := &http.Client{}
	indices, err := listIndices(ctx, httpClient, server.URL, credentials, "test-prefix")

	require.NoError(t, err)
	assert.Len(t, indices, 2, "Should return 2 indices (excluding system index)")
	assert.Contains(t, indices, "test-prefix-product")
	assert.Contains(t, indices, "test-prefix-category")
	assert.NotContains(t, indices, ".system-index")
}

func TestListIndices_NoIndicesFound(t *testing.T) {
	// Mock opensearch server returning 404
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	}))
	defer server.Close()

	ctx := context.Background()
	credentials := &opensearchCredentials{
		Username: "testuser",
		Password: "testpass",
	}

	httpClient := &http.Client{}
	indices, err := listIndices(ctx, httpClient, server.URL, credentials, "nonexistent")

	require.NoError(t, err)
	assert.Empty(t, indices, "Should return empty list when no indices found")
}

func TestDeleteIndex(t *testing.T) {
	deleted := false
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "/test-index", r.URL.Path)
		assert.Equal(t, "DELETE", r.Method)

		// Check basic auth
		username, password, ok := r.BasicAuth()
		assert.True(t, ok)
		assert.Equal(t, "testuser", username)
		assert.Equal(t, "testpass", password)

		deleted = true
		w.WriteHeader(http.StatusOK)
		json.NewEncoder(w).Encode(map[string]interface{}{
			"acknowledged": true,
		})
	}))
	defer server.Close()

	ctx := context.Background()
	credentials := &opensearchCredentials{
		Username: "testuser",
		Password: "testpass",
	}

	httpClient := &http.Client{}
	err := deleteIndex(ctx, httpClient, server.URL, credentials, "test-index")

	require.NoError(t, err)
	assert.True(t, deleted, "Index should have been deleted")
}

func TestDeleteIndex_NotFound(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	}))
	defer server.Close()

	ctx := context.Background()
	credentials := &opensearchCredentials{
		Username: "testuser",
		Password: "testpass",
	}

	httpClient := &http.Client{}
	err := deleteIndex(ctx, httpClient, server.URL, credentials, "nonexistent")

	require.NoError(t, err, "Should not error when deleting non-existent index")
}

func TestGetOpensearchCredentials(t *testing.T) {
	ctx := context.Background()
	scheme := runtime.NewScheme()
	_ = corev1.AddToScheme(scheme)
	_ = v1.AddToScheme(scheme)

	secret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "opensearch-secret",
			Namespace: "default",
		},
		Data: map[string][]byte{
			"password": []byte("mysecretpassword"),
		},
	}

	store := &v1.Store{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-store",
			Namespace: "default",
		},
		Spec: v1.StoreSpec{
			OpensearchSpec: v1.OpensearchSpec{
				Enabled:  true,
				Username: "admin",
				PasswordSecretRef: v1.SecretRef{
					Name: "opensearch-secret",
					Key:  "password",
				},
			},
		},
	}

	client := fake.NewClientBuilder().
		WithScheme(scheme).
		WithObjects(secret).
		Build()

	credentials, err := getOpensearchCredentials(ctx, client, store)

	require.NoError(t, err)
	assert.Equal(t, "admin", credentials.Username)
	assert.Equal(t, "mysecretpassword", credentials.Password)
}

func TestGetOpensearchCredentials_SecretNotFound(t *testing.T) {
	ctx := context.Background()
	scheme := runtime.NewScheme()
	_ = corev1.AddToScheme(scheme)
	_ = v1.AddToScheme(scheme)

	store := &v1.Store{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-store",
			Namespace: "default",
		},
		Spec: v1.StoreSpec{
			OpensearchSpec: v1.OpensearchSpec{
				Enabled:  true,
				Username: "admin",
				PasswordSecretRef: v1.SecretRef{
					Name: "nonexistent-secret",
					Key:  "password",
				},
			},
		},
	}

	client := fake.NewClientBuilder().WithScheme(scheme).Build()

	_, err := getOpensearchCredentials(ctx, client, store)

	require.Error(t, err)
	assert.Contains(t, err.Error(), "failed to get opensearch password secret")
}

func TestCleanupResources_FullFlow(t *testing.T) {
	// Mock opensearch server with index listing and deletion
	deletedIndices := []string{}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Check basic auth
		username, password, ok := r.BasicAuth()
		if !ok || username != "admin" || password != "testpassword" {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}

		switch r.Method {
		case "GET":
			// List indices
			if r.URL.Path == "/_cat/indices/testprefix*" {
				indices := []map[string]interface{}{
					{"index": "testprefix-product"},
					{"index": "testprefix-category"},
				}
				w.Header().Set("Content-Type", "application/json")
				json.NewEncoder(w).Encode(indices)
			} else if r.URL.Path == "/_cat/indices/testprefix-admin*" {
				indices := []map[string]interface{}{
					{"index": "testprefix-admin-user"},
				}
				w.Header().Set("Content-Type", "application/json")
				json.NewEncoder(w).Encode(indices)
			} else {
				w.WriteHeader(http.StatusNotFound)
			}
		case "DELETE":
			// Delete index
			indexName := r.URL.Path[1:] // Remove leading /
			deletedIndices = append(deletedIndices, indexName)
			w.WriteHeader(http.StatusOK)
			json.NewEncoder(w).Encode(map[string]interface{}{
				"acknowledged": true,
			})
		default:
			w.WriteHeader(http.StatusMethodNotAllowed)
		}
	}))
	defer server.Close()

	ctx := context.Background()
	scheme := runtime.NewScheme()
	_ = corev1.AddToScheme(scheme)
	_ = v1.AddToScheme(scheme)

	secret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "opensearch-secret",
			Namespace: "default",
		},
		Data: map[string][]byte{
			"password": []byte("testpassword"),
		},
	}

	// Parse server URL to get host and port
	serverURL, err := url.Parse(server.URL)
	require.NoError(t, err)
	
	portInt, err := strconv.Atoi(serverURL.Port())
	require.NoError(t, err)

	store := &v1.Store{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-store",
			Namespace: "default",
		},
		Spec: v1.StoreSpec{
			OpensearchSpec: v1.OpensearchSpec{
				Enabled:           true,
				CleanupOnDeletion: true,
				Host:              serverURL.Hostname(),
				Port:              int32(portInt),
				Schema:            serverURL.Scheme,
				Username:          "admin",
				PasswordSecretRef: v1.SecretRef{
					Name: "opensearch-secret",
					Key:  "password",
				},
				Index: v1.OpensearchIndexSpec{
					Prefix: "testprefix",
				},
			},
		},
	}

	client := fake.NewClientBuilder().
		WithScheme(scheme).
		WithObjects(secret).
		Build()

	cleanupErr := CleanupResources(ctx, client, store)

	require.NoError(t, cleanupErr)
	// Verify all indices were deleted (2 regular + 1 admin)
	assert.Len(t, deletedIndices, 3)
	assert.Contains(t, deletedIndices, "testprefix-product")
	assert.Contains(t, deletedIndices, "testprefix-category")
	assert.Contains(t, deletedIndices, "testprefix-admin-user")
}

func TestCleanupResources_PartialFailure(t *testing.T) {
	// Mock opensearch server that fails to delete one index
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		username, password, ok := r.BasicAuth()
		if !ok || username != "admin" || password != "testpassword" {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}

		switch r.Method {
		case "GET":
			// List indices
			indices := []map[string]interface{}{
				{"index": "testprefix-product"},
				{"index": "testprefix-category"},
			}
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(indices)
		case "DELETE":
			// Fail to delete testprefix-category
			if r.URL.Path == "/testprefix-category" {
				w.WriteHeader(http.StatusInternalServerError)
				json.NewEncoder(w).Encode(map[string]interface{}{
					"error": "Internal server error",
				})
			} else {
				w.WriteHeader(http.StatusOK)
				json.NewEncoder(w).Encode(map[string]interface{}{
					"acknowledged": true,
				})
			}
		}
	}))
	defer server.Close()

	ctx := context.Background()
	scheme := runtime.NewScheme()
	_ = corev1.AddToScheme(scheme)
	_ = v1.AddToScheme(scheme)

	secret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "opensearch-secret",
			Namespace: "default",
		},
		Data: map[string][]byte{
			"password": []byte("testpassword"),
		},
	}

	serverURL, err := url.Parse(server.URL)
	require.NoError(t, err)
	
	portInt, err := strconv.Atoi(serverURL.Port())
	require.NoError(t, err)

	store := &v1.Store{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-store",
			Namespace: "default",
		},
		Spec: v1.StoreSpec{
			OpensearchSpec: v1.OpensearchSpec{
				Enabled:           true,
				CleanupOnDeletion: true,
				Host:              serverURL.Hostname(),
				Port:              int32(portInt),
				Schema:            serverURL.Scheme,
				Username:          "admin",
				PasswordSecretRef: v1.SecretRef{
					Name: "opensearch-secret",
					Key:  "password",
				},
				Index: v1.OpensearchIndexSpec{
					Prefix: "testprefix",
				},
			},
		},
	}

	client := fake.NewClientBuilder().
		WithScheme(scheme).
		WithObjects(secret).
		Build()

	cleanupErr := CleanupResources(ctx, client, store)

	require.Error(t, cleanupErr)
	assert.Contains(t, cleanupErr.Error(), "failed to delete")
}
