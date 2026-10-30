package snapshot

import (
	"context"
	"sync/atomic"
	"testing"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/minio/minio-go/v7/pkg/credentials"
	"github.com/stretchr/testify/require"
)

type countingCreds struct {
	n atomic.Int64
}

func (c *countingCreds) Retrieve(ctx context.Context) (aws.Credentials, error) {
	c.n.Add(1)
	return aws.Credentials{
		AccessKeyID:     "AKIA",
		SecretAccessKey: "secret",
		SessionToken:    "token-v2",
		CanExpire:       true,
		Expires:         time.Now().Add(time.Hour),
		Source:          "test",
	}, ctx.Err()
}

func TestAWSSDKRefreshProviderAsksSDKAgain(t *testing.T) {
	src := &countingCreds{}
	provider := &awsSDKRefreshProvider{provider: src}

	require.True(t, provider.IsExpired(), "minio must treat provider as expired so it re-retrieves")

	first, err := provider.Retrieve()
	require.NoError(t, err)
	require.Equal(t, "token-v2", first.SessionToken)
	require.Equal(t, credentials.SignatureV4, first.SignerType)

	second, err := provider.Retrieve()
	require.NoError(t, err)
	require.Equal(t, first.AccessKeyID, second.AccessKeyID)
	require.Equal(t, int64(2), src.n.Load(), "refresh provider must call AWS SDK on every Retrieve")
}
