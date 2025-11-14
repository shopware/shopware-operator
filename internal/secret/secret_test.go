package secret

import (
	"context"
	"testing"

	v1 "github.com/shopware/shopware-operator/api/v1"
	"github.com/shopware/shopware-operator/internal/util"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

type mockEventRecorder struct {
	events []string
}

func (m *mockEventRecorder) Event(object runtime.Object, eventType, reason, message string) {
	m.events = append(m.events, eventType+":"+reason+":"+message)
}

func createTestStore() *v1.Store {
	return &v1.Store{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-store",
			Namespace: "default",
		},
		Spec: v1.StoreSpec{
			SecretName: "test-store-secret",
			Database: v1.DatabaseSpec{
				Host:    "db.example.com",
				Port:    3306,
				Version: "8.0",
				User:    "shopware",
				Name:    "shopware",
				SSLMode: "PREFERRED",
				PasswordSecretRef: v1.SecretRef{
					Name: "db-secret",
					Key:  "password",
				},
			},
			AdminCredentials: v1.Credentials{
				Username: "admin",
			},
		},
	}
}

func createDBSecret() *corev1.Secret {
	return &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "db-secret",
			Namespace: "default",
		},
		Data: map[string][]byte{
			"password": []byte("db-password"),
		},
	}
}

func TestEnsureStoreSecret_Success(t *testing.T) {
	ctx := context.Background()
	store := createTestStore()
	dbSecret := createDBSecret()

	scheme := runtime.NewScheme()
	_ = v1.AddToScheme(scheme)
	_ = corev1.AddToScheme(scheme)

	fakeClient := fake.NewClientBuilder().
		WithScheme(scheme).
		WithObjects(store, dbSecret).
		Build()

	recorder := &mockEventRecorder{}

	storeSecret, err := EnsureStoreSecret(ctx, fakeClient, recorder, store)
	if err != nil {
		t.Fatalf("EnsureStoreSecret failed: %v", err)
	}

	if storeSecret == nil {
		t.Fatal("Expected storeSecret to be non-nil")
	}

	// Verify basic fields
	if storeSecret.Name != "test-store-secret" {
		t.Errorf("Expected secret name 'test-store-secret', got '%s'", storeSecret.Name)
	}

	if storeSecret.Namespace != "default" {
		t.Errorf("Expected namespace 'default', got '%s'", storeSecret.Namespace)
	}

	// Verify secret data contains required fields
	requiredKeys := []string{"app-secret", "admin-password", "jwt-private-key", "jwt-public-key", "database-url", "database-password", "database-user", "database-host"}
	for _, key := range requiredKeys {
		if _, exists := storeSecret.Data[key]; !exists {
			t.Errorf("Expected secret to contain key '%s'", key)
		}
	}

	// Verify no events were recorded
	if len(recorder.events) != 0 {
		t.Errorf("Expected no events, got %d events: %v", len(recorder.events), recorder.events)
	}
}

func TestEnsureStoreSecret_WithOpensearch(t *testing.T) {
	ctx := context.Background()
	store := createTestStore()
	store.Spec.OpensearchSpec = v1.OpensearchSpec{
		Enabled:  true,
		Host:     "opensearch.example.com",
		Username: "admin",
		Port:     9200,
		Schema:   "https",
		PasswordSecretRef: v1.SecretRef{
			Name: "opensearch-secret",
			Key:  "password",
		},
	}

	dbSecret := createDBSecret()
	opensearchSecret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "opensearch-secret",
			Namespace: "default",
		},
		Data: map[string][]byte{
			"password": []byte("opensearch-password"),
		},
	}

	scheme := runtime.NewScheme()
	_ = v1.AddToScheme(scheme)
	_ = corev1.AddToScheme(scheme)

	fakeClient := fake.NewClientBuilder().
		WithScheme(scheme).
		WithObjects(store, dbSecret, opensearchSecret).
		Build()

	recorder := &mockEventRecorder{}

	storeSecret, err := EnsureStoreSecret(ctx, fakeClient, recorder, store)
	if err != nil {
		t.Fatalf("EnsureStoreSecret failed: %v", err)
	}

	if storeSecret == nil {
		t.Fatal("Expected storeSecret to be non-nil")
	}

	// Verify opensearch URL is present
	if _, exists := storeSecret.Data["opensearch-url"]; !exists {
		t.Error("Expected secret to contain 'opensearch-url'")
	}

	// Verify no events were recorded
	if len(recorder.events) != 0 {
		t.Errorf("Expected no events, got %d events: %v", len(recorder.events), recorder.events)
	}
}

func TestEnsureStoreSecret_WithFastly(t *testing.T) {
	ctx := context.Background()
	store := createTestStore()
	store.Spec.ShopConfiguration = v1.Configuration{
		Currency: "EUR",
		Locale:   "en-GB",
		Fastly: v1.FastlySpec{
			ServiceRef: v1.SecretRef{
				Name: "fastly-secret",
				Key:  "service-id",
			},
			TokenRef: v1.SecretRef{
				Name: "fastly-secret",
				Key:  "token",
			},
		},
	}

	dbSecret := createDBSecret()
	fastlySecret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "fastly-secret",
			Namespace: "default",
		},
		Data: map[string][]byte{
			"token":      []byte("fastly-api-token"),
			"service-id": []byte("fastly-service-id"),
		},
	}

	scheme := runtime.NewScheme()
	_ = v1.AddToScheme(scheme)
	_ = corev1.AddToScheme(scheme)

	fakeClient := fake.NewClientBuilder().
		WithScheme(scheme).
		WithObjects(store, dbSecret, fastlySecret).
		Build()

	recorder := &mockEventRecorder{}

	storeSecret, err := EnsureStoreSecret(ctx, fakeClient, recorder, store)
	if err != nil {
		t.Fatalf("EnsureStoreSecret failed: %v", err)
	}

	if storeSecret == nil {
		t.Fatal("Expected storeSecret to be non-nil")
	}

	// Verify fastly token is present
	if fastlyToken, exists := storeSecret.Data["fastly-token"]; !exists {
		t.Error("Expected secret to contain 'fastly-token'")
	} else if string(fastlyToken) != "fastly-api-token" {
		t.Errorf("Expected fastly token 'fastly-api-token', got '%s'", string(fastlyToken))
	}

	// Verify no events were recorded
	if len(recorder.events) != 0 {
		t.Errorf("Expected no events, got %d events: %v", len(recorder.events), recorder.events)
	}
}

func TestEnsureStoreSecret_MissingDatabaseSecret(t *testing.T) {
	ctx := context.Background()
	store := createTestStore()

	scheme := runtime.NewScheme()
	_ = v1.AddToScheme(scheme)
	_ = corev1.AddToScheme(scheme)

	fakeClient := fake.NewClientBuilder().
		WithScheme(scheme).
		WithObjects(store).
		Build()

	recorder := &mockEventRecorder{}

	storeSecret, err := EnsureStoreSecret(ctx, fakeClient, recorder, store)
	if err == nil {
		t.Fatal("Expected error when database secret is missing")
	}

	if storeSecret != nil {
		t.Fatal("Expected storeSecret to be nil when database secret is missing")
	}

	// Verify warning event was recorded
	if len(recorder.events) != 1 {
		t.Fatalf("Expected 1 event, got %d events: %v", len(recorder.events), recorder.events)
	}

	// Check that the event starts with the expected prefix (the exact error message may vary)
	expectedPrefix := "Warning:DB secret not found:Missing database secret for Store test-store in namespace default:"
	if len(recorder.events[0]) < len(expectedPrefix) || recorder.events[0][:len(expectedPrefix)] != expectedPrefix {
		t.Errorf("Expected event to start with '%s', got: %s", expectedPrefix, recorder.events[0])
	}
}

func TestEnsureStoreSecret_MissingOpensearchSecret(t *testing.T) {
	ctx := context.Background()
	store := createTestStore()
	store.Spec.OpensearchSpec = v1.OpensearchSpec{
		Enabled:  true,
		Host:     "opensearch.example.com",
		Username: "admin",
		PasswordSecretRef: v1.SecretRef{
			Name: "opensearch-secret",
			Key:  "password",
		},
	}

	dbSecret := createDBSecret()

	scheme := runtime.NewScheme()
	_ = v1.AddToScheme(scheme)
	_ = corev1.AddToScheme(scheme)

	fakeClient := fake.NewClientBuilder().
		WithScheme(scheme).
		WithObjects(store, dbSecret).
		Build()

	recorder := &mockEventRecorder{}

	storeSecret, err := EnsureStoreSecret(ctx, fakeClient, recorder, store)
	if err == nil {
		t.Fatal("Expected error when opensearch secret is missing")
	}

	if storeSecret != nil {
		t.Fatal("Expected storeSecret to be nil when opensearch secret is missing")
	}

	// Verify warning event was recorded
	if len(recorder.events) != 1 {
		t.Fatalf("Expected 1 event, got %d events: %v", len(recorder.events), recorder.events)
	}

	if recorder.events[0] != "Warning:Opensearch secret not found:Missing opensearch secret for Store test-store in namespace default" {
		t.Errorf("Unexpected event: %s", recorder.events[0])
	}
}

func TestEnsureStoreSecret_MissingFastlySecret(t *testing.T) {
	ctx := context.Background()
	store := createTestStore()
	store.Spec.ShopConfiguration = v1.Configuration{
		Currency: "EUR",
		Locale:   "en-GB",
		Fastly: v1.FastlySpec{
			ServiceRef: v1.SecretRef{
				Name: "fastly-secret",
				Key:  "service-id",
			},
			TokenRef: v1.SecretRef{
				Name: "fastly-secret",
				Key:  "token",
			},
		},
	}

	dbSecret := createDBSecret()

	scheme := runtime.NewScheme()
	_ = v1.AddToScheme(scheme)
	_ = corev1.AddToScheme(scheme)

	fakeClient := fake.NewClientBuilder().
		WithScheme(scheme).
		WithObjects(store, dbSecret).
		Build()

	recorder := &mockEventRecorder{}

	storeSecret, err := EnsureStoreSecret(ctx, fakeClient, recorder, store)
	if err == nil {
		t.Fatal("Expected error when fastly secret is missing")
	}

	if storeSecret != nil {
		t.Fatal("Expected storeSecret to be nil when fastly secret is missing")
	}

	// Verify warning event was recorded
	if len(recorder.events) != 1 {
		t.Fatalf("Expected 1 event, got %d events: %v", len(recorder.events), recorder.events)
	}

	if recorder.events[0] != "Warning:Fastly secret not found:Missing Fastly secret for Store test-store in namespace default" {
		t.Errorf("Unexpected event: %s", recorder.events[0])
	}
}

func TestEnsureStoreSecret_MissingDatabaseHost(t *testing.T) {
	ctx := context.Background()
	store := createTestStore()
	store.Spec.Database.Host = ""

	scheme := runtime.NewScheme()
	_ = v1.AddToScheme(scheme)
	_ = corev1.AddToScheme(scheme)

	fakeClient := fake.NewClientBuilder().
		WithScheme(scheme).
		WithObjects(store).
		Build()

	recorder := &mockEventRecorder{}

	_, err := EnsureStoreSecret(ctx, fakeClient, recorder, store)
	if err == nil {
		t.Fatal("Expected error for missing database host")
	}

	expectedError := "database host is empty for store test-store. Either set host or a hostRef"
	if err.Error() != expectedError {
		t.Errorf("Expected error '%s', got '%s'", expectedError, err.Error())
	}
}

func TestEnsureStoreSecret_OpensearchEnabledButMissingConfig(t *testing.T) {
	ctx := context.Background()
	store := createTestStore()
	store.Spec.OpensearchSpec = v1.OpensearchSpec{
		Enabled: true,
		Host:    "opensearch.example.com",
		// Missing PasswordSecretRef
	}

	scheme := runtime.NewScheme()
	_ = v1.AddToScheme(scheme)
	_ = corev1.AddToScheme(scheme)

	fakeClient := fake.NewClientBuilder().
		WithScheme(scheme).
		WithObjects(store).
		Build()

	recorder := &mockEventRecorder{}

	_, err := EnsureStoreSecret(ctx, fakeClient, recorder, store)
	if err == nil {
		t.Fatal("Expected error for opensearch enabled without secret config")
	}

	expectedError := "opensearch is enabled but passwordSecretRef key or name is empty for store test-store"
	if err.Error() != expectedError {
		t.Errorf("Expected error '%s', got '%s'", expectedError, err.Error())
	}
}

func TestEnsureStoreSecret_FastlyServiceIDWithoutSecret(t *testing.T) {
	ctx := context.Background()
	store := createTestStore()
	store.Spec.ShopConfiguration = v1.Configuration{
		Currency: "EUR",
		Locale:   "en-GB",
		Fastly: v1.FastlySpec{
			ServiceRef: v1.SecretRef{
				Name: "fastly-secret",
				Key:  "service-id",
			},
			// Missing TokenRef
		},
	}

	scheme := runtime.NewScheme()
	_ = v1.AddToScheme(scheme)
	_ = corev1.AddToScheme(scheme)

	fakeClient := fake.NewClientBuilder().
		WithScheme(scheme).
		WithObjects(store).
		Build()

	recorder := &mockEventRecorder{}

	_, err := EnsureStoreSecret(ctx, fakeClient, recorder, store)
	if err == nil {
		t.Fatal("Expected error for fastly serviceID without secret config")
	}

	expectedError := "fastly apiTokenSecretRef key or name is empty for store test-store. Unset the fastlyServiceID or provide a secret"
	if err.Error() != expectedError {
		t.Errorf("Expected error '%s', got '%s'", expectedError, err.Error())
	}
}

func TestGenerateStoreSecret(t *testing.T) {
	ctx := context.Background()
	store := createTestStore()

	dbSpec := &util.DatabaseSpec{
		Host:     "db.example.com",
		Password: []byte("db-password"),
		User:     "shopware",
		Port:     3306,
		Name:     "shopware",
		Version:  "8.0",
		SSLMode:  "PREFERRED",
	}

	secret := &corev1.Secret{
		Data: make(map[string][]byte),
	}

	opensearchPassword := []byte("opensearch-password")
	fastlyServiceID := []byte("fastly-service-id")
	fastlyToken := []byte("fastly-token")

	err := GenerateStoreSecret(ctx, store, secret, dbSpec, opensearchPassword, fastlyServiceID, fastlyToken)
	if err != nil {
		t.Fatalf("GenerateStoreSecret failed: %v", err)
	}

	// Verify all required fields are present
	requiredKeys := []string{"app-secret", "admin-password", "jwt-private-key", "jwt-public-key", "database-url", "database-password", "database-user", "database-host", "fastly-token"}
	for _, key := range requiredKeys {
		if _, exists := secret.Data[key]; !exists {
			t.Errorf("Expected secret to contain key '%s'", key)
		}
	}

	// Verify app-secret is 128 characters
	if len(secret.Data["app-secret"]) != 128 {
		t.Errorf("Expected app-secret to be 128 characters, got %d", len(secret.Data["app-secret"]))
	}

	// Verify admin-password is 20 characters (since we didn't set a custom password)
	if len(secret.Data["admin-password"]) != 20 {
		t.Errorf("Expected admin-password to be 20 characters, got %d", len(secret.Data["admin-password"]))
	}

	// Verify fastly token
	if string(secret.Data["fastly-token"]) != "fastly-token" {
		t.Errorf("Expected fastly-token to be 'fastly-token', got '%s'", string(secret.Data["fastly-token"]))
	}

	// Verify database fields
	if string(secret.Data["database-user"]) != "shopware" {
		t.Errorf("Expected database-user to be 'shopware', got '%s'", string(secret.Data["database-user"]))
	}

	if string(secret.Data["database-host"]) != "db.example.com" {
		t.Errorf("Expected database-host to be 'db.example.com', got '%s'", string(secret.Data["database-host"]))
	}
}

func TestGenerateStoreSecret_CustomAdminPassword(t *testing.T) {
	ctx := context.Background()
	store := createTestStore()
	store.Spec.AdminCredentials.Password = "custom-admin-password"

	dbSpec := &util.DatabaseSpec{
		Host:     "db.example.com",
		Password: []byte("db-password"),
		User:     "shopware",
		Port:     3306,
		Name:     "shopware",
		Version:  "8.0",
		SSLMode:  "PREFERRED",
	}

	secret := &corev1.Secret{
		Data: make(map[string][]byte),
	}

	err := GenerateStoreSecret(ctx, store, secret, dbSpec, nil, nil, nil)
	if err != nil {
		t.Fatalf("GenerateStoreSecret failed: %v", err)
	}

	// Verify custom admin password is used
	if string(secret.Data["admin-password"]) != "custom-admin-password" {
		t.Errorf("Expected admin-password to be 'custom-admin-password', got '%s'", string(secret.Data["admin-password"]))
	}
}

func TestGenerateStoreSecret_PreservesExistingSecrets(t *testing.T) {
	ctx := context.Background()
	store := createTestStore()

	dbSpec := &util.DatabaseSpec{
		Host:     "db.example.com",
		Password: []byte("db-password"),
		User:     "shopware",
		Port:     3306,
		Name:     "shopware",
		Version:  "8.0",
		SSLMode:  "PREFERRED",
	}

	secret := &corev1.Secret{
		Data: map[string][]byte{
			"app-secret":      []byte("existing-app-secret"),
			"admin-password":  []byte("existing-admin-password"),
			"jwt-private-key": []byte("existing-jwt-private-key"),
			"jwt-public-key":  []byte("existing-jwt-public-key"),
		},
	}

	err := GenerateStoreSecret(ctx, store, secret, dbSpec, nil, nil, nil)
	if err != nil {
		t.Fatalf("GenerateStoreSecret failed: %v", err)
	}

	// Verify existing secrets are preserved
	if string(secret.Data["app-secret"]) != "existing-app-secret" {
		t.Error("Expected app-secret to be preserved")
	}

	if string(secret.Data["admin-password"]) != "existing-admin-password" {
		t.Error("Expected admin-password to be preserved")
	}

	if string(secret.Data["jwt-private-key"]) != "existing-jwt-private-key" {
		t.Error("Expected jwt-private-key to be preserved")
	}

	if string(secret.Data["jwt-public-key"]) != "existing-jwt-public-key" {
		t.Error("Expected jwt-public-key to be preserved")
	}
}

func TestEnsureStoreSecret_WithDatabaseHostRef(t *testing.T) {
	ctx := context.Background()
	store := createTestStore()
	store.Spec.Database.Host = ""
	store.Spec.Database.HostRef = v1.SecretRef{
		Name: "db-host-secret",
		Key:  "host",
	}

	dbSecret := createDBSecret()
	dbHostSecret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "db-host-secret",
			Namespace: "default",
		},
		Data: map[string][]byte{
			"host": []byte("db-from-secret.example.com"),
		},
	}

	scheme := runtime.NewScheme()
	_ = v1.AddToScheme(scheme)
	_ = corev1.AddToScheme(scheme)

	fakeClient := fake.NewClientBuilder().
		WithScheme(scheme).
		WithObjects(store, dbSecret, dbHostSecret).
		Build()

	recorder := &mockEventRecorder{}

	storeSecret, err := EnsureStoreSecret(ctx, fakeClient, recorder, store)
	if err != nil {
		t.Fatalf("EnsureStoreSecret failed: %v", err)
	}

	if storeSecret == nil {
		t.Fatal("Expected storeSecret to be non-nil")
	}

	// Verify database host from secret is used
	if string(storeSecret.Data["database-host"]) != "db-from-secret.example.com" {
		t.Errorf("Expected database-host to be 'db-from-secret.example.com', got '%s'", string(storeSecret.Data["database-host"]))
	}

	// Verify no events were recorded
	if len(recorder.events) != 0 {
		t.Errorf("Expected no events, got %d events: %v", len(recorder.events), recorder.events)
	}
}

func TestEnsureStoreSecret_UpdateExistingSecret(t *testing.T) {
	ctx := context.Background()
	store := createTestStore()
	dbSecret := createDBSecret()

	// Existing store secret with some data
	existingStoreSecret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-store-secret",
			Namespace: "default",
		},
		Data: map[string][]byte{
			"app-secret":     []byte("existing-app-secret"),
			"admin-password": []byte("existing-admin-password"),
		},
	}

	scheme := runtime.NewScheme()
	_ = v1.AddToScheme(scheme)
	_ = corev1.AddToScheme(scheme)

	fakeClient := fake.NewClientBuilder().
		WithScheme(scheme).
		WithObjects(store, dbSecret, existingStoreSecret).
		Build()

	recorder := &mockEventRecorder{}

	storeSecret, err := EnsureStoreSecret(ctx, fakeClient, recorder, store)
	if err != nil {
		t.Fatalf("EnsureStoreSecret failed: %v", err)
	}

	if storeSecret == nil {
		t.Fatal("Expected storeSecret to be non-nil")
	}

	// Verify existing secrets are preserved
	if string(storeSecret.Data["app-secret"]) != "existing-app-secret" {
		t.Error("Expected existing app-secret to be preserved")
	}

	if string(storeSecret.Data["admin-password"]) != "existing-admin-password" {
		t.Error("Expected existing admin-password to be preserved")
	}

	// Verify new fields are added
	if _, exists := storeSecret.Data["database-url"]; !exists {
		t.Error("Expected database-url to be added")
	}

	// Verify the secret was actually retrieved from the fake client
	var retrievedSecret corev1.Secret
	err = fakeClient.Get(ctx, types.NamespacedName{Name: "test-store-secret", Namespace: "default"}, &retrievedSecret)
	if err != nil {
		t.Fatalf("Failed to retrieve secret from fake client: %v", err)
	}
}
