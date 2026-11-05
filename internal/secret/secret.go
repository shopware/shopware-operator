package secret

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/base64"
	"encoding/pem"
	"fmt"
	"math/big"
	"net/url"
	"time"

	"github.com/pkg/errors"
	v1 "github.com/shopware/shopware-operator/api/v1"
	"github.com/shopware/shopware-operator/internal/util"
	corev1 "k8s.io/api/core/v1"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

const (
	passSymbols = "ABCDEFGHIJKLMNOPQRSTUVWXYZ" +
		"abcdefghijklmnopqrstuvwxyz" +
		"0123456789"
	rsaKeySize = 2024
)

type referencedValues struct {
	OpensearchPassword []byte
	FastlyServiceID    []byte
	FastlyToken        []byte
	AdminUsername      []byte
	AdminPassword      []byte
	TidewaysAPIKey     []byte
	S3AccessKeyID      []byte
	S3SecretAccessKey  []byte
}

type referencedSecret struct {
	ref    v1.SecretRef
	label  string
	target *[]byte
}

func EnsureStoreSecret(ctx context.Context, r client.Client, store *v1.Store) (*corev1.Secret, error) {
	values, err := readReferencedValues(ctx, r, store)
	if err != nil {
		return nil, err
	}

	dbSpec, err := util.GetDBSpec(ctx, *store, r)
	if err != nil {
		if k8serrors.IsNotFound(err) {
			return nil, newDatabaseSecretNotFoundError(store, err)
		}
		return nil, fmt.Errorf("can't read database secret: %w", err)
	}

	nn := types.NamespacedName{
		Namespace: store.Namespace,
		Name:      store.GetSecretName(),
	}

	storeSecret := new(corev1.Secret)
	if err = r.Get(ctx, nn, storeSecret); client.IgnoreNotFound(err) != nil {
		return nil, fmt.Errorf("get store secret: %w", err)
	}

	if err = generateStoreSecret(store, storeSecret, dbSpec, values); err != nil {
		return nil, fmt.Errorf("fill store secret: %w", err)
	}
	storeSecret.Name = store.GetSecretName()
	storeSecret.Namespace = store.Namespace

	return storeSecret, nil
}

func readReferencedValues(ctx context.Context, r client.Client, store *v1.Store) (referencedValues, error) {
	var values referencedValues
	spec := store.Spec

	refs := []referencedSecret{
		{ref: spec.AdminCredentials.UsernameSecretRef, label: "Admin credentials", target: &values.AdminUsername},
		{ref: spec.AdminCredentials.PasswordSecretRef, label: "Admin credentials", target: &values.AdminPassword},
		{ref: spec.S3Storage.AccessKeyRef, label: "S3 access key", target: &values.S3AccessKeyID},
		{ref: spec.S3Storage.SecretAccessKeyRef, label: "S3 secret access key", target: &values.S3SecretAccessKey},
	}
	if spec.Tideways.Enabled {
		refs = append(refs, referencedSecret{ref: spec.Tideways.APIKeyRef, label: "Tideways", target: &values.TidewaysAPIKey})
	}
	if spec.OpensearchSpec.Enabled {
		refs = append(refs, referencedSecret{ref: spec.OpensearchSpec.PasswordSecretRef, label: "Opensearch", target: &values.OpensearchPassword})
	}
	if spec.ShopConfiguration.Fastly.TokenRef.IsSet() && spec.ShopConfiguration.Fastly.ServiceRef.IsSet() {
		refs = append(refs,
			referencedSecret{ref: spec.ShopConfiguration.Fastly.TokenRef, label: "Fastly", target: &values.FastlyToken},
			referencedSecret{ref: spec.ShopConfiguration.Fastly.ServiceRef, label: "Fastly", target: &values.FastlyServiceID},
		)
	}

	for _, ref := range refs {
		if !ref.ref.IsSet() {
			continue
		}
		value, err := getSecretValue(ctx, r, store, ref.ref, ref.label)
		if err != nil {
			if k8serrors.IsNotFound(err) {
				return values, newReferencedSecretNotFoundError(store, ref.label, err)
			}
			return values, err
		}
		*ref.target = value
	}

	return values, nil
}

func setOrDelete(data map[string][]byte, key string, value []byte) {
	if len(value) == 0 {
		delete(data, key)
		return
	}
	data[key] = value
}

func generateStoreSecret(store *v1.Store, secret *corev1.Secret, dbSpec *util.DatabaseSpec, values referencedValues) error {
	if secret.Data == nil {
		secret.Data = make(map[string][]byte)
	}

	if _, ok := secret.Data["app-secret"]; !ok {
		pass, err := generatePass(128)
		if err != nil {
			return fmt.Errorf("generate app secret: %w", err)
		}
		secret.Data["app-secret"] = pass
	}

	if store.Spec.AdminCredentials.PasswordSecretRef.IsSet() {
		secret.Data["admin-password"] = values.AdminPassword
	} else if _, ok := secret.Data["admin-password"]; !ok {
		if store.Spec.AdminCredentials.Password != "" {
			secret.Data["admin-password"] = []byte(store.Spec.AdminCredentials.Password)
		} else {
			admin, err := generatePass(20)
			if err != nil {
				return fmt.Errorf("generate admin secret: %w", err)
			}
			secret.Data["admin-password"] = admin
		}
	}

	if store.Spec.AdminCredentials.UsernameSecretRef.IsSet() {
		secret.Data["admin-username"] = values.AdminUsername
	} else {
		setOrDelete(secret.Data, "admin-username", []byte(store.Spec.AdminCredentials.Username))
	}

	if _, ok := secret.Data["jwt-private-key"]; !ok {
		privateKey, publicKey, err := generatePrivatePublicKey(rsaKeySize)
		if err != nil {
			return fmt.Errorf("create jwt keys: %w", err)
		}
		secret.Data["jwt-private-key"] = []byte(base64.StdEncoding.EncodeToString(privateKey))
		secret.Data["jwt-public-key"] = []byte(base64.StdEncoding.EncodeToString(publicKey))
	}

	// Used for snapshot controller
	secret.Data["database-password"] = []byte(url.QueryEscape(string(dbSpec.Password)))
	secret.Data["database-user"] = []byte(dbSpec.User)
	secret.Data["database-host"] = []byte(dbSpec.Host)

	secret.Data["database-url"] = util.GenerateDatabaseURLForShopware(dbSpec)

	if store.Spec.OpensearchSpec.Enabled {
		secret.Data["opensearch-url"] = util.GenerateOpensearchURLForShopware(&store.Spec.OpensearchSpec, values.OpensearchPassword)
	} else {
		delete(secret.Data, "opensearch-url")
	}

	if len(values.FastlyServiceID) > 0 && len(values.FastlyToken) > 0 {
		secret.Data["fastly-token"] = values.FastlyToken
		secret.Data["fastly-service-id"] = values.FastlyServiceID
	} else {
		delete(secret.Data, "fastly-token")
		delete(secret.Data, "fastly-service-id")
	}

	setOrDelete(secret.Data, "tideways-api-key", values.TidewaysAPIKey)
	setOrDelete(secret.Data, "s3-access-key-id", values.S3AccessKeyID)
	setOrDelete(secret.Data, "s3-secret-access-key", values.S3SecretAccessKey)

	return nil
}

func getSecretValue(ctx context.Context, r client.Client, store *v1.Store, ref v1.SecretRef, description string) ([]byte, error) {
	secret := new(corev1.Secret)
	if err := r.Get(ctx, types.NamespacedName{Namespace: store.Namespace, Name: ref.Name}, secret); err != nil {
		if k8serrors.IsNotFound(err) {
			return nil, fmt.Errorf("can't find %s secret %s: %w", description, ref.Name, err)
		}
		return nil, fmt.Errorf("can't read %s secret %s: %w", description, ref.Name, err)
	}

	value, ok := secret.Data[ref.Key]
	if !ok {
		return nil, fmt.Errorf("%s secret %s does not contain key %s", description, ref.Name, ref.Key)
	}
	return value, nil
}

func generatePass(long int) ([]byte, error) {
	b := make([]byte, long)
	for i := 0; i != long; i++ {
		randInt, err := rand.Int(rand.Reader, big.NewInt(int64(len(passSymbols))))
		if err != nil {
			return nil, errors.Wrap(err, "get rand int")
		}
		b[i] = passSymbols[randInt.Int64()]
	}

	return b, nil
}

func generatePrivatePublicKey(keyLength int) ([]byte, []byte, error) {
	rsaKey, err := rsa.GenerateKey(rand.Reader, keyLength)
	if err != nil {
		return nil, nil, err
	}

	rsaPrivKey := encodePrivateKeyToPEM(rsaKey)
	rsaPubKey, err := generatePublicKey(rsaKey)
	if err != nil {
		return nil, nil, fmt.Errorf("unable to generate public key: %v", err)
	}

	return rsaPrivKey, rsaPubKey, nil
}

func encodePrivateKeyToPEM(privateKey *rsa.PrivateKey) []byte {
	key := &pem.Block{
		Type:  "RSA PRIVATE KEY",
		Bytes: x509.MarshalPKCS1PrivateKey(privateKey),
	}

	return pem.EncodeToMemory(key)
}

func generatePublicKey(key *rsa.PrivateKey) ([]byte, error) {
	template := x509.Certificate{
		SerialNumber: big.NewInt(1),
		Subject: pkix.Name{
			Organization: []string{"Acme Co"},
		},
		NotBefore: time.Now(),
		NotAfter:  time.Now().Add(time.Hour * 24 * 180),

		KeyUsage:              x509.KeyUsageKeyEncipherment | x509.KeyUsageDigitalSignature,
		ExtKeyUsage:           []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
		BasicConstraintsValid: true,
	}

	derBytes, err := x509.CreateCertificate(rand.Reader, &template, &template, &key.PublicKey, key)
	if err != nil {
		return nil, err
	}

	pubPem := &pem.Block{Type: "CERTIFICATE", Bytes: derBytes}

	return pem.EncodeToMemory(pubPem), nil
}
