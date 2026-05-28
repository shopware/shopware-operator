package snapshot

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestJoinSnapshotErrorsDropsCanceledWhenPrimaryErrorExists(t *testing.T) {
	primary := errors.New("database backup failed")
	err := joinSnapshotErrors([]error{primary, context.Canceled})

	require.Error(t, err)
	assert.ErrorIs(t, err, primary)
	assert.NotErrorIs(t, err, context.Canceled)
}

func TestJoinSnapshotErrorsKeepsCanceledWhenItIsOnlyError(t *testing.T) {
	err := joinSnapshotErrors([]error{context.Canceled})

	require.Error(t, err)
	assert.ErrorIs(t, err, context.Canceled)
}
