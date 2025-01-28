package util

import (
	"context"
	"fmt"
	"net/url"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/minio/minio-go/v7"
	"github.com/minio/minio-go/v7/pkg/credentials"
	v1 "github.com/shopware/shopware-operator/api/v1"
)

func TestS3Connection(ctx context.Context, s v1.S3Storage, c aws.Credentials) error {
	url, err := url.Parse(s.EndpointURL)
	if err != nil {
		return fmt.Errorf("parsing url failed: %w", err)
	}

	endpoint := fmt.Sprintf("%s:%s", url.Hostname(), url.Port())
	s3Client, err := minio.New(endpoint, &minio.Options{
		Creds:  credentials.NewStaticV4(c.AccessKeyID, c.SecretAccessKey, ""),
		Secure: url.Scheme == "https",
	})
	if err != nil {
		return err
	}

	buckets, err := s3Client.ListBuckets(ctx)
	if err != nil {
		return fmt.Errorf("listing buckets failed: %w", err)
	}

	var public bool
	var private bool
	for _, b := range buckets {
		if b.Name == s.PublicBucketName {
			public = true
		}
		if b.Name != s.PrivateBucketName {
			private = true
		}
	}

	if public && private {
		return nil
	}
	return fmt.Errorf("public bucket is not available yet")
}
