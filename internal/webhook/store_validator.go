package webhook

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	shopv1 "github.com/shopware/shopware-operator/api/v1"
	"github.com/shopware/shopware-operator/internal/logging"
	"go.uber.org/zap"
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
type StoreValidator struct {
	Logger *zap.SugaredLogger
}

func (validator StoreValidator) Handle(ctx context.Context, req admission.Request) (response admission.Response) {
	logger := validator.Logger
	if logger == nil {
		logger = logging.FromContext(ctx)
	}
	logger.Infow("Store validation webhook invoked",
		"operation", req.Operation,
		"namespace", req.Namespace,
		"name", req.Name,
		"group", req.Resource.Group,
		"version", req.Resource.Version,
		"resource", req.Resource.Resource,
		"subresource", req.SubResource,
	)
	defer func() {
		fields := []interface{}{"allowed", response.Allowed}
		if response.Result != nil {
			fields = append(fields,
				"reason", response.Result.Reason,
				"code", response.Result.Code,
				"message", response.Result.Message,
			)
		}
		logger.Infow("Store validation webhook completed", fields...)
	}()

	if req.Operation != admissionv1.Create && req.Operation != admissionv1.Update {
		return admission.Allowed("")
	}

	errors := validateContainerOverrides(req.Object.Raw)
	if len(errors) > 0 && strings.HasPrefix(errors[0], "invalid Store JSON:") {
		return admission.Denied(errors[0])
	}
	errors = append(errors, validateExplicitOverrideRules(req.Object.Raw)...)

	if len(errors) > 0 {
		return admission.Denied("invalid container override: " + strings.Join(errors, "; "))
	}

	return admission.Allowed("")
}

func validateContainerOverrides(raw []byte) []string {
	var object map[string]json.RawMessage
	strictErrors, err := kjson.UnmarshalStrict(raw, &object, kjson.DisallowDuplicateFields)
	if err != nil {
		return []string{fmt.Sprintf("invalid Store JSON: %v", err)}
	}

	var errors []string
	for _, strictErr := range strictErrors {
		errors = append(errors, strictErr.Error())
	}

	specRaw, found := object["spec"]
	if !found || len(specRaw) == 0 {
		return errors
	}

	var spec map[string]json.RawMessage
	specStrictErrors, err := kjson.UnmarshalStrict(specRaw, &spec, kjson.DisallowDuplicateFields)
	if err != nil {
		return append(errors, fmt.Sprintf("invalid Store JSON: %v", err))
	}
	for _, strictErr := range specStrictErrors {
		errors = append(errors, strictErr.Error())
	}

	for _, fieldName := range storeContainerOverrideFields {
		overrideRaw, found := spec[fieldName]
		if !found {
			continue
		}

		var override shopv1.ContainerMergeSpec
		strictErrors, err := kjson.UnmarshalStrict(
			overrideRaw,
			&override,
			kjson.DisallowDuplicateFields,
			kjson.DisallowUnknownFields,
		)
		if err != nil {
			errors = append(errors, err.Error())
		}
		for _, strictErr := range strictErrors {
			errors = append(errors, strictErr.Error())
		}
	}

	return errors
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
