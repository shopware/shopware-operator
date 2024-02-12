package util_test

import (
	"context"
	"testing"

	v1 "github.com/shopware/shopware-operator/api/v1"
	"github.com/shopware/shopware-operator/internal/util"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/stretchr/testify/assert"
)

func TestS3Errors(t *testing.T) {
	err := util.TestS3Connection(context.Background(), v1.S3Storage{}, aws.Credentials{
		SecretAccessKey: "",
		SessionToken:    "",
	})
	assert.ErrorContains(t, err, "")
}
