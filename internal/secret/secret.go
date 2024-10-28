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
	"time"

	"github.com/pkg/errors"
	v1 "github.com/shopware/shopware-operator/api/v1"
	"github.com/shopware/shopware-operator/internal/util"
	corev1 "k8s.io/api/core/v1"
)

const (
	passSymbols = "ABCDEFGHIJKLMNOPQRSTUVWXYZ" +
		"abcdefghijklmnopqrstuvwxyz" +
		"0123456789"
	rsaKeySize = 2024
)

func GenerateStoreSecret(ctx context.Context, store *v1.Store, secret *corev1.Secret, dbHost string, p []byte) error {
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

	secret.Data["database-url"] = util.GenerateDatabaseURLForShopware(&store.Spec.Database, dbHost, p)

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
