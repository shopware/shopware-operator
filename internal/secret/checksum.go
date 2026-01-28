package secret

import (
	"context"
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"fmt"
	"hash"
	"slices"

	v1 "github.com/shopware/shopware-operator/api/v1"
	corev1 "k8s.io/api/core/v1"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

const ChecksumAnnotation = "shopware.com/secret-checksum"

func SecretsChecksum(ctx context.Context, cl client.Client, store *v1.Store) (string, error) {
	s := new(corev1.Secret)
	if err := cl.Get(ctx, types.NamespacedName{Namespace: store.Namespace, Name: store.GetSecretName()}, s); err != nil {
		if k8serrors.IsNotFound(err) {
			return "", nil
		}
		return "", fmt.Errorf("get store secret for checksum: %w", err)
	}
	return dataChecksum(s.Data), nil
}

func dataChecksum(data map[string][]byte) string {
	keys := make([]string, 0, len(data))
	for key := range data {
		keys = append(keys, key)
	}
	slices.Sort(keys)

	h := sha256.New()
	for _, key := range keys {
		writeField(h, []byte(key))
		writeField(h, data[key])
	}
	return hex.EncodeToString(h.Sum(nil))
}

func writeField(h hash.Hash, value []byte) {
	_ = binary.Write(h, binary.BigEndian, uint64(len(value)))
	_, _ = h.Write(value)
}
