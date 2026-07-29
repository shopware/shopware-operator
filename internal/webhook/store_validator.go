package webhook

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	shopv1 "github.com/shopware/shopware-operator/api/v1"
	admissionv1 "k8s.io/api/admission/v1"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"
	kjson "sigs.k8s.io/json"
)

const StoreValidationPath = "/validate-shop-shopware-com-v1-store"

var storeContainerOverrideFields = []string{
	"adminDeploymentContainer",
	"workerDeploymentContainer",
	"storefrontDeploymentContainer",
	"setupJobContainer",
	"migrationJobContainer",
}

// +kubebuilder:webhook:path=/validate-shop-shopware-com-v1-store,mutating=false,failurePolicy=fail,sideEffects=None,groups=shop.shopware.com,resources=stores,verbs=create;update,versions=v1,name=vstore.shopware.com,admissionReviewVersions=v1

// StoreValidator validates the schemaless container override fields against
// their strongly typed Go representation.
type StoreValidator struct{}

func (StoreValidator) Handle(_ context.Context, req admission.Request) admission.Response {
	if req.Operation != admissionv1.Create && req.Operation != admissionv1.Update {
		return admission.Allowed("")
	}

	var store shopv1.Store
	strictErrors, err := kjson.UnmarshalStrict(
		req.Object.Raw,
		&store,
		kjson.DisallowDuplicateFields,
		kjson.DisallowUnknownFields,
	)
	if err != nil {
		return admission.Denied(fmt.Sprintf("invalid Store JSON: %v", err))
	}

	errors := make([]string, 0, len(strictErrors)+1)
	for _, strictErr := range strictErrors {
		errors = append(errors, strictErr.Error())
	}
	errors = append(errors, validateExplicitOverrideRules(req.Object.Raw)...)

	if len(errors) > 0 {
		return admission.Denied("invalid container override: " + strings.Join(errors, "; "))
	}

	return admission.Allowed("")
}

func validateExplicitOverrideRules(raw []byte) []string {
	var object struct {
		Spec map[string]json.RawMessage `json:"spec"`
	}
	if err := json.Unmarshal(raw, &object); err != nil {
		return nil // The strict decoder reports syntax and type errors.
	}

	var errors []string
	for _, fieldName := range storeContainerOverrideFields {
		overrideRaw, found := object.Spec[fieldName]
		if !found {
			continue
		}

		var override map[string]json.RawMessage
		if err := json.Unmarshal(overrideRaw, &override); err != nil {
			continue // The strict decoder reports a non-object value.
		}

		imageRaw, found := override["image"]
		if !found {
			continue
		}

		var image string
		if err := json.Unmarshal(imageRaw, &image); err == nil && image == "" {
			errors = append(errors, fmt.Sprintf("spec.%s.image must not be empty", fieldName))
		}
	}

	return errors
}
