package snapshot

import (
	"context"
	"errors"
	"fmt"
	"os"
	"syscall"
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

func TestIsNoSpaceErrorDetectsWrappedENOSPC(t *testing.T) {
	copyErr := fmt.Errorf("failed to copy object to file: %w", &os.PathError{
		Op:   "write",
		Path: "/snapshot-storage/snapshot-3539155404/public/thumbnail/0c/e1/b6/1793133547/26306999_1_1_1920x1920.jpg",
		Err:  syscall.ENOSPC,
	})
	err := fmt.Errorf("public bucket backup: error processing object thumbnail/0c/e1/b6/1793133547/26306999_1_1_1920x1920.jpg: %w", copyErr)

	assert.True(t, IsNoSpaceError(err))
	assert.False(t, IsNoSpaceError(errors.New("the provided token has expired")))
}

func TestJoinSnapshotErrorsKeepsCanceledWhenItIsOnlyError(t *testing.T) {
	err := joinSnapshotErrors([]error{context.Canceled})

	require.Error(t, err)
	assert.ErrorIs(t, err, context.Canceled)
}
