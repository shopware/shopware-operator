package webhook

import (
	"context"
	"fmt"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	admissionv1 "k8s.io/api/admission/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"
)

func TestStoreValidatorAcceptsContainerOverrides(t *testing.T) {
	t.Parallel()

	for _, fieldName := range storeContainerOverrideFields {
		t.Run(fieldName, func(t *testing.T) {
			t.Parallel()

			raw := fmt.Sprintf(`{
  "apiVersion":"shop.shopware.com/v1",
  "kind":"Store",
  "metadata":{"name":"test","namespace":"default"},
  "spec":{"%s":{"image":"example.com/shopware:latest","replicas":2,"extraEnvs":[{"name":"APP_ENV","value":"prod"}],"resources":{"requests":{"cpu":"100m"}}}}
}`, fieldName)

			response := validateStore(t, admissionv1.Create, raw)
			require.True(t, response.Allowed, response.Result.Message)
		})
	}
}

func TestStoreValidatorRejectsInvalidOverrides(t *testing.T) {
	t.Parallel()

	tests := map[string]struct {
		raw     string
		message string
	}{
		"unknown field": {
			raw:     `{"spec":{"workerDeploymentContainer":{"unknownField":true}}}`,
			message: "unknown field",
		},
		"unknown nested field": {
			raw:     `{"spec":{"workerDeploymentContainer":{"extraEnvs":[{"name":"APP_ENV","unknownField":true}]}}}`,
			message: "unknown field",
		},
		"duplicate field": {
			raw:     `{"spec":{"workerDeploymentContainer":{"replicas":1,"replicas":2}}}`,
			message: "duplicate field",
		},
		"wrong field type": {
			raw:     `{"spec":{"workerDeploymentContainer":{"replicas":"two"}}}`,
			message: "cannot unmarshal",
		},
		"empty image": {
			raw:     `{"spec":{"workerDeploymentContainer":{"image":""}}}`,
			message: "spec.workerDeploymentContainer.image must not be empty",
		},
		"malformed JSON": {
			raw:     `{"spec":`,
			message: "invalid Store JSON",
		},
	}

	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			response := validateStore(t, admissionv1.Update, test.raw)
			require.False(t, response.Allowed)
			assert.Contains(t, strings.ToLower(response.Result.Message), strings.ToLower(test.message))
		})
	}
}

func TestStoreValidatorIgnoresOtherOperations(t *testing.T) {
	t.Parallel()

	response := validateStore(t, admissionv1.Delete, `{"not":"a store"}`)
	require.True(t, response.Allowed, response.Result.Message)
}

func validateStore(t *testing.T, operation admissionv1.Operation, raw string) admission.Response {
	t.Helper()

	return (StoreValidator{}).Handle(context.Background(), admission.Request{
		AdmissionRequest: admissionv1.AdmissionRequest{
			Operation: operation,
			Object: runtime.RawExtension{
				Raw: []byte(raw),
			},
		},
	})
}
