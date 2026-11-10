package secret

import (
	"fmt"

	v1 "github.com/shopware/shopware-operator/api/v1"
	corev1 "k8s.io/api/core/v1"
)

type EventError struct {
	EventType string
	Reason    string
	Message   string
	Err       error
}

func (e *EventError) Error() string {
	return e.Err.Error()
}

func (e *EventError) Unwrap() error {
	return e.Err
}

func NewWarningError(reason, message string, err error) *EventError {
	return &EventError{
		EventType: corev1.EventTypeWarning,
		Reason:    reason,
		Message:   message,
		Err:       err,
	}
}

func newDatabaseSecretNotFoundError(store *v1.Store, err error) *EventError {
	return NewWarningError(
		"DB secret not found",
		fmt.Sprintf("Missing database secret for Store %s in namespace %s: %s", store.Name, store.Namespace, err.Error()),
		fmt.Errorf("can't find database secret: %w", err),
	)
}

func newReferencedSecretNotFoundError(store *v1.Store, label string, err error) *EventError {
	return NewWarningError(
		label+" secret not found",
		fmt.Sprintf("Missing %s secret for Store %s in namespace %s", label, store.Name, store.Namespace),
		err,
	)
}
