package util

import (
	"crypto/md5"
	"crypto/sha256"
	"errors"
	"fmt"
	"strings"

	v1 "github.com/shopware/shopware-operator/api/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/validation"
)

const (
	SnapshotSubCommandCreate  = "create"
	SnapshotSubCommandRestore = "restore"

	MaxNameLength        = 63
	MaxCronJobNameLength = 52
	nameHashLength       = 8

	kedaHPAPrefix = "keda-hpa-"
)

func ShortName(name string) string {
	return ShortNameWithLimit(name, MaxNameLength)
}

func ShortNameWithLimit(name string, limit int) string {
	if len(name) <= limit {
		return name
	}
	hash := fmt.Sprintf("%x", sha256.Sum256([]byte(name)))[:nameHashLength]
	return strings.TrimRight(name[:limit-nameHashLength-1], "-.") + "-" + hash
}

func GetAdminDeploymentName(store v1.Store) string {
	return fmt.Sprintf("%s-store-admin", store.Name)
}

func GetStorefrontDeploymentName(store v1.Store) string {
	return fmt.Sprintf("%s-storefront", store.Name)
}

func GetWorkerDeploymentName(store v1.Store) string {
	return fmt.Sprintf("%s-store-worker", store.Name)
}

func GetQueueWorkerDeploymentName(store v1.Store, queue string) string {
	if queue == "" {
		return GetWorkerDeploymentName(store)
	}
	sanitized := strings.ReplaceAll(strings.ToLower(queue), "_", "-")
	if sanitized != queue {
		sanitized += "-" + fmt.Sprintf("%x", sha256.Sum256([]byte(queue)))[:nameHashLength]
	}
	return ShortName(fmt.Sprintf("%s-%s", GetWorkerDeploymentName(store), sanitized))
}

func GetWorkerScaledObjectHPAName(scaledObjectName string) string {
	defaultName := kedaHPAPrefix + scaledObjectName
	if len(defaultName) <= MaxNameLength {
		return ""
	}
	return ShortName(defaultName)
}

func GetAdminServiceName(store v1.Store) string {
	return ShortName(fmt.Sprintf("%s-admin", store.Name))
}

func GetStorefrontServiceName(store v1.Store) string {
	return ShortName(fmt.Sprintf("%s-storefront", store.Name))
}

func GetStorefrontPDBName(store v1.Store) string {
	return fmt.Sprintf("%s-storefront", store.Name)
}

func GetAdminPDBName(store v1.Store) string {
	return fmt.Sprintf("%s-admin", store.Name)
}

func GetWorkerPDBName(store v1.Store) string {
	return fmt.Sprintf("%s-worker", store.Name)
}

func GetStoreHPAName(store v1.Store) string {
	return fmt.Sprintf("store-%s", store.Name)
}

func GetStoreIngressName(store v1.Store) string {
	return fmt.Sprintf("store-%s", store.Name)
}

func GetStoreHTTPRouteName(store v1.Store) string {
	return fmt.Sprintf("store-%s", store.Name)
}

func GetSetupJobName(store v1.Store) string {
	return ShortName(fmt.Sprintf("%s-setup", store.Name))
}

func MigrateJobName(store v1.Store) string {
	return ShortName(fmt.Sprintf("%s-migrate-%s", store.Name, GetMigrateHash(store)))
}

func GetMigrateHash(store v1.Store) string {
	return fmt.Sprintf("%x", md5.Sum([]byte(store.Status.CurrentImageTag)))
}

func GetScheduledCronJobName(store v1.Store) string {
	return ShortNameWithLimit(fmt.Sprintf("%s-scheduled-jobs", store.Name), MaxCronJobNameLength)
}

func CommandJobName(exec v1.StoreExec) string {
	return exec.Name
}

func SnapshotJobName(snap metav1.ObjectMeta, subCommand string) string {
	return ShortName(fmt.Sprintf("snapshot-%s-%s", subCommand, snap.Name))
}

func GetDebugPodName(instance v1.StoreDebugInstance) string {
	return instance.Name
}

func GetDebugServiceName(instance v1.StoreDebugInstance) string {
	return instance.Name
}

type nameCheck struct {
	kind     string
	name     string
	validate func(string) []string
}

func ValidateStoreNames(store v1.Store) error {
	checks := []nameCheck{
		{"store name label", store.Name, validation.IsValidLabelValue},
		{"admin deployment", GetAdminDeploymentName(store), validation.IsDNS1123Subdomain},
		{"storefront deployment", GetStorefrontDeploymentName(store), validation.IsDNS1123Subdomain},
		{"worker deployment", GetWorkerDeploymentName(store), validation.IsDNS1123Subdomain},
		{"admin service", GetAdminServiceName(store), validation.IsDNS1035Label},
		{"storefront service", GetStorefrontServiceName(store), validation.IsDNS1035Label},
		{"storefront pdb", GetStorefrontPDBName(store), validation.IsDNS1123Subdomain},
		{"admin pdb", GetAdminPDBName(store), validation.IsDNS1123Subdomain},
		{"worker pdb", GetWorkerPDBName(store), validation.IsDNS1123Subdomain},
		{"hpa", GetStoreHPAName(store), validation.IsDNS1123Subdomain},
		{"ingress", GetStoreIngressName(store), validation.IsDNS1123Subdomain},
		{"httproute", GetStoreHTTPRouteName(store), validation.IsDNS1123Subdomain},
		{"setup job", GetSetupJobName(store), validation.IsDNS1123Label},
		{"migration job", MigrateJobName(store), validation.IsDNS1123Label},
		{"scheduled cronjob", GetScheduledCronJobName(store), validateCronJobName},
	}

	for _, transport := range store.Status.QueueState.Transports {
		name := GetQueueWorkerDeploymentName(store, transport.Name)
		checks = append(checks,
			nameCheck{"queue worker deployment", name, validation.IsDNS1123Subdomain},
			nameCheck{"queue label", ShortName(transport.Name), validation.IsValidLabelValue},
		)
		if hpaName := GetWorkerScaledObjectHPAName(name); hpaName != "" {
			checks = append(checks, nameCheck{"keda hpa", hpaName, validation.IsDNS1123Subdomain})
		}
	}

	return runNameChecks(checks)
}

func ValidateStoreExecNames(exec v1.StoreExec) error {
	return runNameChecks([]nameCheck{
		{"store exec name label", exec.Name, validation.IsValidLabelValue},
		{"command job", CommandJobName(exec), validation.IsDNS1123Label},
	})
}

func ValidateSnapshotNames(name string, subCommand string) error {
	return runNameChecks([]nameCheck{
		{"snapshot name label", name, validation.IsValidLabelValue},
		{"snapshot job", SnapshotJobName(metav1.ObjectMeta{Name: name}, subCommand), validation.IsDNS1123Label},
	})
}

func ValidateDebugInstanceNames(instance v1.StoreDebugInstance) error {
	return runNameChecks([]nameCheck{
		{"debug instance name label", instance.Name, validation.IsValidLabelValue},
		{"debug pod", GetDebugPodName(instance), validation.IsDNS1123Subdomain},
		{"debug service", GetDebugServiceName(instance), validation.IsDNS1035Label},
	})
}

func validateCronJobName(name string) []string {
	errs := validation.IsDNS1123Subdomain(name)
	if len(name) > MaxCronJobNameLength {
		errs = append(errs, validation.MaxLenError(MaxCronJobNameLength))
	}
	return errs
}

func runNameChecks(checks []nameCheck) error {
	var errs []error
	for _, check := range checks {
		for _, msg := range check.validate(check.name) {
			errs = append(errs, fmt.Errorf("invalid %s name %q: %s", check.kind, check.name, msg))
		}
	}
	return errors.Join(errs...)
}
