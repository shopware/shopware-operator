package snapshot

import (
	"context"
	"errors"
)

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
