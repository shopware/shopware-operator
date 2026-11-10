package util

import (
	"crypto/md5"
	"crypto/sha256"
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

func shortNameWithLimit(name string, limit int) string {
	return shortenWithHash(name, limit, "-.")
}

func shortLabelValue(value string) string {
	return shortenWithHash(value, validation.LabelValueMaxLength, "-")
}

func shortenWithHash(value string, limit int, trimSuffix string) string {
	if len(value) <= limit {
		return value
	}
	hash := fmt.Sprintf("%x", sha256.Sum256([]byte(value)))[:nameHashLength]
	return strings.TrimRight(value[:limit-nameHashLength-1], trimSuffix) + "-" + hash
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
	return shortNameWithLimit(fmt.Sprintf("%s-%s", GetWorkerDeploymentName(store), sanitized), MaxNameLength)
}

func GetQueueLabelValue(queue string) string {
	return shortLabelValue(queue)
}

func GetQueuesLabelValue(queues []string) string {
	return shortLabelValue(strings.Join(queues, "."))
}

func GetWorkerScaledObjectHPAName(scaledObjectName string) string {
	defaultName := kedaHPAPrefix + scaledObjectName
	if len(defaultName) <= MaxNameLength {
		return ""
	}
	return shortNameWithLimit(defaultName, MaxNameLength)
}

func GetAdminServiceName(store v1.Store) string {
	return shortNameWithLimit(fmt.Sprintf("%s-admin", store.Name), MaxNameLength)
}

func GetStorefrontServiceName(store v1.Store) string {
	return shortNameWithLimit(fmt.Sprintf("%s-storefront", store.Name), MaxNameLength)
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
	return shortNameWithLimit(fmt.Sprintf("%s-setup", store.Name), MaxNameLength)
}

func MigrateJobName(store v1.Store) string {
	return shortNameWithLimit(fmt.Sprintf("%s-migrate-%s", store.Name, GetMigrateHash(store)), MaxNameLength)
}

func GetMigrateHash(store v1.Store) string {
	return fmt.Sprintf("%x", md5.Sum([]byte(store.Status.CurrentImageTag)))
}

func GetScheduledCronJobName(store v1.Store) string {
	return shortNameWithLimit(fmt.Sprintf("%s-scheduled-jobs", store.Name), MaxCronJobNameLength)
}

func CommandJobName(exec v1.StoreExec) string {
	return exec.Name
}

func CommandCronJobName(exec v1.StoreExec) string {
	return shortNameWithLimit(exec.Name, MaxCronJobNameLength)
}

func SnapshotJobName(snap metav1.ObjectMeta, subCommand string) string {
	return shortNameWithLimit(fmt.Sprintf("snapshot-%s-%s", subCommand, snap.Name), MaxNameLength)
}

func GetDebugPodName(instance v1.StoreDebugInstance) string {
	return instance.Name
}

func GetDebugServiceName(instance v1.StoreDebugInstance) string {
	return instance.Name
}
