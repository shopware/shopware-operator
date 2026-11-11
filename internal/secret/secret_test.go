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
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

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

	storeSecret, err := EnsureStoreSecret(ctx, fakeClient, store)
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
}

func TestEnsureStoreSecret_AdminCredentialsSecretRefs(t *testing.T) {
	ctx := context.Background()
	store := createTestStore()
	store.Spec.AdminCredentials.Username = ""
	store.Spec.AdminCredentials.Password = ""
	store.Spec.AdminCredentials.UsernameSecretRef = v1.SecretRef{Name: "admin-credentials", Key: "username"}
	store.Spec.AdminCredentials.PasswordSecretRef = v1.SecretRef{Name: "admin-credentials", Key: "password"}
	dbSecret := createDBSecret()
	adminSecret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{Name: "admin-credentials", Namespace: "default"},
		Data:       map[string][]byte{"username": []byte("secret-admin"), "password": []byte("secret-password")},
	}

	scheme := runtime.NewScheme()
	_ = v1.AddToScheme(scheme)
	_ = corev1.AddToScheme(scheme)
	fakeClient := fake.NewClientBuilder().WithScheme(scheme).WithObjects(store, dbSecret, adminSecret).Build()

	storeSecret, err := EnsureStoreSecret(ctx, fakeClient, store)
	if err != nil {
		t.Fatalf("EnsureStoreSecret failed: %v", err)
	}
	if got := string(storeSecret.Data["admin-password"]); got != "secret-password" {
		t.Fatalf("expected referenced admin password, got %q", got)
	}
}

func TestEnsureStoreSecret_AdminCredentialsPasswordSecretRefUpdatesExistingSecret(t *testing.T) {
	ctx := context.Background()
	store := createTestStore()
	store.Spec.AdminCredentials.Password = ""
	store.Spec.AdminCredentials.PasswordSecretRef = v1.SecretRef{Name: "admin-credentials", Key: "password"}
	dbSecret := createDBSecret()
	adminSecret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{Name: "admin-credentials", Namespace: "default"},
		Data:       map[string][]byte{"password": []byte("rotated-password")},
	}
	storeSecret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{Name: store.GetSecretName(), Namespace: store.Namespace},
		Data:       map[string][]byte{"admin-password": []byte("old-password")},
	}

	scheme := runtime.NewScheme()
	_ = v1.AddToScheme(scheme)
	_ = corev1.AddToScheme(scheme)
	fakeClient := fake.NewClientBuilder().WithScheme(scheme).WithObjects(store, dbSecret, adminSecret, storeSecret).Build()

	result, err := EnsureStoreSecret(ctx, fakeClient, store)
	if err != nil {
		t.Fatalf("EnsureStoreSecret failed: %v", err)
	}
	if got := string(result.Data["admin-password"]); got != "rotated-password" {
		t.Fatalf("expected referenced admin password to replace existing value, got %q", got)
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

	storeSecret, err := EnsureStoreSecret(ctx, fakeClient, store)
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

	storeSecret, err := EnsureStoreSecret(ctx, fakeClient, store)
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

	storeSecret, err := EnsureStoreSecret(ctx, fakeClient, store)
	if err == nil {
		t.Fatal("Expected error when database secret is missing")
	}

	if storeSecret != nil {
		t.Fatal("Expected storeSecret to be nil when database secret is missing")
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

	storeSecret, err := EnsureStoreSecret(ctx, fakeClient, store)
	if err == nil {
		t.Fatal("Expected error when opensearch secret is missing")
	}

	if storeSecret != nil {
		t.Fatal("Expected storeSecret to be nil when opensearch secret is missing")
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

	storeSecret, err := EnsureStoreSecret(ctx, fakeClient, store)
	if err == nil {
		t.Fatal("Expected error when fastly secret is missing")
	}

	if storeSecret != nil {
		t.Fatal("Expected storeSecret to be nil when fastly secret is missing")
	}
}

func TestGenerateStoreSecret(t *testing.T) {
	store := createTestStore()

	dbSpec := &util.DatabaseSpec{
		Host:     "db.example.com",
		Password: []byte("db-password"),
		User:     "shopware",
		Port:     3306,
		Name:     "shopware",
		Version:  "8.0",
	}

	secret := &corev1.Secret{
		Data: make(map[string][]byte),
	}

	opensearchPassword := []byte("opensearch-password")
	fastlyServiceID := []byte("fastly-service-id")
	fastlyToken := []byte("fastly-token")

	err := generateStoreSecret(store, secret, dbSpec, referencedValues{OpensearchPassword: opensearchPassword, FastlyServiceID: fastlyServiceID, FastlyToken: fastlyToken})
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
	store := createTestStore()
	store.Spec.AdminCredentials.Password = "custom-admin-password"

	dbSpec := &util.DatabaseSpec{
		Host:     "db.example.com",
		Password: []byte("db-password"),
		User:     "shopware",
		Port:     3306,
		Name:     "shopware",
		Version:  "8.0",
	}

	secret := &corev1.Secret{
		Data: make(map[string][]byte),
	}

	err := generateStoreSecret(store, secret, dbSpec, referencedValues{})
	if err != nil {
		t.Fatalf("GenerateStoreSecret failed: %v", err)
	}

	// Verify custom admin password is used
	if string(secret.Data["admin-password"]) != "custom-admin-password" {
		t.Errorf("Expected admin-password to be 'custom-admin-password', got '%s'", string(secret.Data["admin-password"]))
	}
}

func TestGenerateStoreSecret_PreservesExistingSecrets(t *testing.T) {
	store := createTestStore()

	dbSpec := &util.DatabaseSpec{
		Host:     "db.example.com",
		Password: []byte("db-password"),
		User:     "shopware",
		Port:     3306,
		Name:     "shopware",
		Version:  "8.0",
	}

	secret := &corev1.Secret{
		Data: map[string][]byte{
			"app-secret":      []byte("existing-app-secret"),
			"admin-password":  []byte("existing-admin-password"),
			"jwt-private-key": []byte("existing-jwt-private-key"),
			"jwt-public-key":  []byte("existing-jwt-public-key"),
		},
	}

	err := generateStoreSecret(store, secret, dbSpec, referencedValues{})
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

	storeSecret, err := EnsureStoreSecret(ctx, fakeClient, store)
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

	storeSecret, err := EnsureStoreSecret(ctx, fakeClient, store)
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

func TestEnsureStoreSecret_CopiesReferencedSecrets(t *testing.T) {
	ctx := context.Background()
	store := createTestStore()
	store.Spec.AdminCredentials.Username = ""
	store.Spec.AdminCredentials.UsernameSecretRef = v1.SecretRef{Name: "admin-credentials", Key: "username"}
	store.Spec.Tideways.Enabled = true
	store.Spec.Tideways.APIKeyRef = v1.SecretRef{Name: "tideways", Key: "api-key"}
	store.Spec.S3Storage.AccessKeyRef = v1.SecretRef{Name: "s3", Key: "access"}
	store.Spec.S3Storage.SecretAccessKeyRef = v1.SecretRef{Name: "s3", Key: "secret"}

	objects := []client.Object{
		store,
		createDBSecret(),
		&corev1.Secret{
			ObjectMeta: metav1.ObjectMeta{Name: "admin-credentials", Namespace: "default"},
			Data:       map[string][]byte{"username": []byte("secret-admin")},
		},
		&corev1.Secret{
			ObjectMeta: metav1.ObjectMeta{Name: "tideways", Namespace: "default"},
			Data:       map[string][]byte{"api-key": []byte("tideways-key")},
		},
		&corev1.Secret{
			ObjectMeta: metav1.ObjectMeta{Name: "s3", Namespace: "default"},
			Data:       map[string][]byte{"access": []byte("access-id"), "secret": []byte("secret-key")},
		},
	}

	scheme := runtime.NewScheme()
	_ = v1.AddToScheme(scheme)
	_ = corev1.AddToScheme(scheme)
	fakeClient := fake.NewClientBuilder().WithScheme(scheme).WithObjects(objects...).Build()

	storeSecret, err := EnsureStoreSecret(ctx, fakeClient, store)
	if err != nil {
		t.Fatalf("EnsureStoreSecret failed: %v", err)
	}

	expected := map[string]string{
		"admin-username":       "secret-admin",
		"tideways-api-key":     "tideways-key",
		"s3-access-key-id":     "access-id",
		"s3-secret-access-key": "secret-key",
	}
	for key, value := range expected {
		if got := string(storeSecret.Data[key]); got != value {
			t.Errorf("expected %s to be %q, got %q", key, value, got)
		}
	}
}

func TestEnsureStoreSecret_RemovesUnreferencedKeys(t *testing.T) {
	ctx := context.Background()
	store := createTestStore()
	existing := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{Name: "test-store-secret", Namespace: "default"},
		Data: map[string][]byte{
			"tideways-api-key":     []byte("old"),
			"s3-access-key-id":     []byte("old"),
			"s3-secret-access-key": []byte("old"),
			"fastly-token":         []byte("old"),
			"fastly-service-id":    []byte("old"),
		},
	}

	scheme := runtime.NewScheme()
	_ = v1.AddToScheme(scheme)
	_ = corev1.AddToScheme(scheme)
	fakeClient := fake.NewClientBuilder().WithScheme(scheme).WithObjects(store, createDBSecret(), existing).Build()

	storeSecret, err := EnsureStoreSecret(ctx, fakeClient, store)
	if err != nil {
		t.Fatalf("EnsureStoreSecret failed: %v", err)
	}

	for key := range existing.Data {
		if _, ok := storeSecret.Data[key]; ok {
			t.Errorf("expected %s to be removed", key)
		}
	}
	if got := string(storeSecret.Data["admin-username"]); got != "admin" {
		t.Errorf("expected admin-username to be %q, got %q", "admin", got)
	}
}

func TestEnsureStoreSecret_IgnoresTidewaysSecretWhenDisabled(t *testing.T) {
	ctx := context.Background()
	store := createTestStore()
	store.Spec.Tideways.Enabled = false
	store.Spec.Tideways.APIKeyRef = v1.SecretRef{Name: "missing-tideways", Key: "api-key"}
	existing := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{Name: "test-store-secret", Namespace: "default"},
		Data:       map[string][]byte{"tideways-api-key": []byte("old")},
	}

	scheme := runtime.NewScheme()
	_ = v1.AddToScheme(scheme)
	_ = corev1.AddToScheme(scheme)
	fakeClient := fake.NewClientBuilder().WithScheme(scheme).WithObjects(store, createDBSecret(), existing).Build()

	storeSecret, err := EnsureStoreSecret(ctx, fakeClient, store)
	if err != nil {
		t.Fatalf("EnsureStoreSecret failed: %v", err)
	}
	if _, ok := storeSecret.Data["tideways-api-key"]; ok {
		t.Error("expected tideways-api-key to be removed when tideways is disabled")
	}
}
