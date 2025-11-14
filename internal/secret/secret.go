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
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

const (
	passSymbols = "ABCDEFGHIJKLMNOPQRSTUVWXYZ" +
		"abcdefghijklmnopqrstuvwxyz" +
		"0123456789"
	rsaKeySize = 2024
)

type EventRecorder interface {
	Event(object runtime.Object, eventType, reason, message string)
}

func EnsureStoreSecret(ctx context.Context, r client.Client, recorder EventRecorder, store *v1.Store) (*corev1.Secret, error) {
	if store.Spec.Database.Host == "" && store.Spec.Database.HostRef.Name == "" {
		return nil, fmt.Errorf("database host is empty for store %s. Either set host or a hostRef", store.Name)
	}

	if store.Spec.OpensearchSpec.Enabled && (store.Spec.OpensearchSpec.PasswordSecretRef.Name == "" || store.Spec.OpensearchSpec.PasswordSecretRef.Key == "") {
		return nil, fmt.Errorf("opensearch is enabled but passwordSecretRef key or name is empty for store %s", store.Name)
	}

	// Validate Fastly configuration: if ServiceRef is provided, TokenRef must also be provided
	if (store.Spec.ShopConfiguration.Fastly.ServiceRef.Name != "" || store.Spec.ShopConfiguration.Fastly.ServiceRef.Key != "") &&
		(store.Spec.ShopConfiguration.Fastly.TokenRef.Name == "" || store.Spec.ShopConfiguration.Fastly.TokenRef.Key == "") {
		return nil, fmt.Errorf("fastly apiTokenSecretRef key or name is empty for store %s. Unset the fastlyServiceID or provide a secret", store.Name)
	}

	var opensearchPassword []byte
	if store.Spec.OpensearchSpec.Enabled {
		es := new(corev1.Secret)
		if err := r.Get(ctx, types.NamespacedName{
			Namespace: store.Namespace,
			Name:      store.Spec.OpensearchSpec.PasswordSecretRef.Name,
		}, es); err != nil {
			if k8serrors.IsNotFound(err) {
				recorder.Event(store, "Warning", "Opensearch secret not found",
					fmt.Sprintf("Missing opensearch secret for Store %s in namespace %s",
						store.Name,
						store.Namespace))
				return nil, fmt.Errorf("can't find opensearch secret: %w", err)
			}
			return nil, fmt.Errorf("can't read opensearch secret: %w", err)
		}
		opensearchPassword = es.Data[store.Spec.OpensearchSpec.PasswordSecretRef.Key]
	}

	var fastlyToken []byte
	if store.Spec.ShopConfiguration.Fastly.TokenRef.Name != "" && store.Spec.ShopConfiguration.Fastly.TokenRef.Key != "" {
		fastlyTokenSecret := new(corev1.Secret)
		if err := r.Get(ctx, types.NamespacedName{
			Namespace: store.Namespace,
			Name:      store.Spec.ShopConfiguration.Fastly.TokenRef.Name,
		}, fastlyTokenSecret); err != nil {
			if k8serrors.IsNotFound(err) {
				recorder.Event(store, "Warning", "Fastly secret not found",
					fmt.Sprintf("Missing Fastly secret for Store %s in namespace %s",
						store.Name,
						store.Namespace))
				return nil, fmt.Errorf("can't find Fastly secret: %w", err)
			}
			return nil, fmt.Errorf("can't read Fastly secret: %w", err)
		}
		fastlyToken = fastlyTokenSecret.Data[store.Spec.ShopConfiguration.Fastly.TokenRef.Key]
	}

	var fastlyServiceID []byte
	if store.Spec.ShopConfiguration.Fastly.ServiceRef.Name != "" && store.Spec.ShopConfiguration.Fastly.ServiceRef.Key != "" {
		fastlyServiceIDSecret := new(corev1.Secret)
		if err := r.Get(ctx, types.NamespacedName{
			Namespace: store.Namespace,
			Name:      store.Spec.ShopConfiguration.Fastly.ServiceRef.Name,
		}, fastlyServiceIDSecret); err != nil {
			if k8serrors.IsNotFound(err) {
				recorder.Event(store, "Warning", "Fastly secret not found",
					fmt.Sprintf("Missing Fastly secret for Store %s in namespace %s",
						store.Name,
						store.Namespace))
				return nil, fmt.Errorf("can't find Fastly secret: %w", err)
			}
			return nil, fmt.Errorf("can't read Fastly secret: %w", err)
		}
		fastlyServiceID = fastlyServiceIDSecret.Data[store.Spec.ShopConfiguration.Fastly.ServiceRef.Key]
	}

	dbSpec, err := util.GetDBSpec(ctx, *store, r)
	if err != nil {
		if k8serrors.IsNotFound(err) {
			recorder.Event(store, "Warning", "DB secret not found",
				fmt.Sprintf("Missing database secret for Store %s in namespace %s: %s",
					store.Name,
					store.Namespace,
					err.Error()))
			return nil, fmt.Errorf("can't find database secret: %w", err)
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

	if err = GenerateStoreSecret(ctx, store, storeSecret, dbSpec, opensearchPassword, fastlyServiceID, fastlyToken); err != nil {
		return nil, fmt.Errorf("fill store secret: %w", err)
	}
	storeSecret.Name = store.GetSecretName()
	storeSecret.Namespace = store.Namespace

	return storeSecret, nil
}

func GenerateStoreSecret(ctx context.Context, store *v1.Store, secret *corev1.Secret, dbSpec *util.DatabaseSpec, opensearchPassword []byte, fastlyServiceID []byte, fastlyToken []byte) error {
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

	if _, ok := secret.Data["admin-password"]; !ok {
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

	if len(opensearchPassword) > 0 {
		secret.Data["opensearch-url"] = util.GenerateOpensearchURLForShopware(&store.Spec.OpensearchSpec, opensearchPassword)
	}

	if len(fastlyServiceID) > 0 && len(fastlyToken) > 0 {
		secret.Data["fastly-token"] = fastlyToken
		secret.Data["fastly-service-id"] = fastlyServiceID
	}

	return nil
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
