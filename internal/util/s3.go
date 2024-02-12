package util

import (
	"context"
	"fmt"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/minio/minio-go/v7"
	"github.com/minio/minio-go/v7/pkg/credentials"
	v1 "github.com/shopware/shopware-operator/api/v1"
)

func TestS3Connection(ctx context.Context, s v1.S3Storage, c aws.Credentials) error {
	s3Client, err := minio.New(s.EndpointURL, &minio.Options{
		Creds:  credentials.NewStaticV4(c.AccessKeyID, c.SecretAccessKey, ""),
		Secure: false,
	})
	if err != nil {
		return err
	}

	buckets, err := s3Client.ListBuckets(ctx)
	if err != nil {
		return fmt.Errorf("listing buckets failed: %w", err)
	}

	if len(buckets) <= 1 {
		return fmt.Errorf("You should have at least two buckets (private and public)")
	}

	for _, b := range buckets {
		if b.Name != s.PublicBucketName && b.Name != s.PrivateBucketName {
			return fmt.Errorf("private or public bucket are not available yet")
		}
	}

	return nil
}
