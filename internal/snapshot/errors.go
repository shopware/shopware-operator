package snapshot

import (
	"context"
	"errors"
	"os"
	"strings"
	"syscall"
)

// ExitCodeNoSpace is used when a snapshot fails because the temp volume is full
// or CloudWatch estimates the S3 payload will not fit. The Job podFailurePolicy
// maps this code to FailJob so Kubernetes does not retry.
const ExitCodeNoSpace = 16

// ErrInsufficientTempSpace is returned when the S3 size precheck estimates the
// snapshot will not fit in --temp-size.
var ErrInsufficientTempSpace = errors.New("insufficient temp storage for snapshot")

func IsNoSpaceError(err error) bool {
	if err == nil {
		return false
	}
	if errors.Is(err, ErrInsufficientTempSpace) {
		return true
	}
	if errors.Is(err, syscall.ENOSPC) {
		return true
	}
	var errno syscall.Errno
	if errors.As(err, &errno) && errno == syscall.ENOSPC {
		return true
	}
	var pathErr *os.PathError
	if errors.As(err, &pathErr) && IsNoSpaceError(pathErr.Err) {
		return true
	}
	return strings.Contains(strings.ToLower(err.Error()), "no space left on device")
}

func joinSnapshotErrors(errs []error) error {
	if len(errs) == 0 {
		return nil
	}

	filtered := make([]error, 0, len(errs))
	hasNonCanceledError := false
	for _, err := range errs {
		if err == nil {
			continue
		}
		if !errors.Is(err, context.Canceled) {
			hasNonCanceledError = true
			filtered = append(filtered, err)
		}
	}

	if hasNonCanceledError {
		return errors.Join(filtered...)
	}

	return errors.Join(errs...)
}
