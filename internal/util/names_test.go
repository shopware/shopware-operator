package util_test

import (
	"strings"
	"testing"

	v1 "github.com/shopware/shopware-operator/api/v1"
	"github.com/shopware/shopware-operator/internal/util"
	"github.com/stretchr/testify/assert"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/validation"
)

func storeWithName(name string) v1.Store {
	return v1.Store{ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: "test"}}
}

func assertValidName(t *testing.T, kind, name string, validate func(string) []string) {
	t.Helper()
	assert.Empty(t, validate(name), "%s name %q", kind, name)
}

func assertMaxLen(t *testing.T, kind, name string, limit int) {
	t.Helper()
	assert.LessOrEqual(t, len(name), limit, "%s name %q", kind, name)
}

func TestStoreNamesStayValidForMaxLengthStoreName(t *testing.T) {
	store := storeWithName(strings.Repeat("a", 63))
	store.Status.CurrentImageTag = "6.6.10.0"

	assertValidName(t, "admin deployment", util.GetAdminDeploymentName(store), validation.IsDNS1123Subdomain)
	assertValidName(t, "storefront deployment", util.GetStorefrontDeploymentName(store), validation.IsDNS1123Subdomain)
	assertValidName(t, "worker deployment", util.GetWorkerDeploymentName(store), validation.IsDNS1123Subdomain)
	assertValidName(t, "storefront pdb", util.GetStorefrontPDBName(store), validation.IsDNS1123Subdomain)
	assertValidName(t, "admin pdb", util.GetAdminPDBName(store), validation.IsDNS1123Subdomain)
	assertValidName(t, "worker pdb", util.GetWorkerPDBName(store), validation.IsDNS1123Subdomain)
	assertValidName(t, "hpa", util.GetStoreHPAName(store), validation.IsDNS1123Subdomain)
	assertValidName(t, "ingress", util.GetStoreIngressName(store), validation.IsDNS1123Subdomain)
	assertValidName(t, "httproute", util.GetStoreHTTPRouteName(store), validation.IsDNS1123Subdomain)

	assertValidName(t, "admin service", util.GetAdminServiceName(store), validation.IsDNS1035Label)
	assertValidName(t, "storefront service", util.GetStorefrontServiceName(store), validation.IsDNS1035Label)

	assertValidName(t, "setup job", util.GetSetupJobName(store), validation.IsDNS1123Subdomain)
	assertMaxLen(t, "setup job", util.GetSetupJobName(store), util.MaxNameLength)
	assertValidName(t, "migration job", util.MigrateJobName(store), validation.IsDNS1123Subdomain)
	assertMaxLen(t, "migration job", util.MigrateJobName(store), util.MaxNameLength)

	assertValidName(t, "scheduled cronjob", util.GetScheduledCronJobName(store), validation.IsDNS1123Subdomain)
	assertMaxLen(t, "scheduled cronjob", util.GetScheduledCronJobName(store), util.MaxCronJobNameLength)
}

func TestShortStoreNamesAreUnchanged(t *testing.T) {
	store := storeWithName("my-store")

	assert.Equal(t, "my-store-admin", util.GetAdminServiceName(store))
	assert.Equal(t, "my-store-storefront", util.GetStorefrontServiceName(store))
	assert.Equal(t, "my-store-setup", util.GetSetupJobName(store))
	assert.Equal(t, "my-store-scheduled-jobs", util.GetScheduledCronJobName(store))
	assert.Equal(t, "my-store-migrate-"+util.GetMigrateHash(store), util.MigrateJobName(store))
}

func TestQueueWorkerNamesStayValid(t *testing.T) {
	store := storeWithName(strings.Repeat("a", 63))

	for _, queue := range []string{"async", "low_priority", "failed.async", strings.Repeat("q", 80)} {
		name := util.GetQueueWorkerDeploymentName(store, queue)
		assertValidName(t, "queue worker deployment", name, validation.IsDNS1123Subdomain)
		assertMaxLen(t, "queue worker deployment", name, util.MaxNameLength)

		hpaName := util.GetWorkerScaledObjectHPAName(name)
		assert.NotEmpty(t, hpaName)
		assertValidName(t, "keda hpa", hpaName, validation.IsDNS1123Label)
		assert.True(t, strings.HasPrefix(hpaName, "keda-hpa-"))
	}
}

func TestWorkerScaledObjectHPANameKeepsKedaDefaultWhenShort(t *testing.T) {
	assert.Empty(t, util.GetWorkerScaledObjectHPAName("my-store-store-worker-async"))
}

func TestQueueLabelValues(t *testing.T) {
	t.Run("long queue label is shortened", func(t *testing.T) {
		label := util.GetQueueLabelValue(strings.Repeat("q", 80))
		assertValidName(t, "queue label", label, validation.IsValidLabelValue)
	})

	t.Run("joined queues label is shortened", func(t *testing.T) {
		queues := []string{strings.Repeat("a", 40), strings.Repeat("b", 40)}
		label := util.GetQueuesLabelValue(queues)
		assertValidName(t, "worker queues label", label, validation.IsValidLabelValue)
	})

	t.Run("short labels are unchanged", func(t *testing.T) {
		assert.Equal(t, "async", util.GetQueueLabelValue("async"))
		assert.Equal(t, "async.low_priority", util.GetQueuesLabelValue([]string{"async", "low_priority"}))
	})

	t.Run("keeps legacy shortening for selector stability", func(t *testing.T) {
		queue := strings.Repeat("a", 53) + "." + strings.Repeat("b", 20)
		label := util.GetQueueLabelValue(queue)
		assert.Len(t, label, 63)
		assert.Equal(t, strings.Repeat("a", 53)+".-", label[:55])
	})
}

func TestCommandJobNames(t *testing.T) {
	exec := v1.StoreExec{ObjectMeta: metav1.ObjectMeta{Name: strings.Repeat("a", 63)}}

	assert.Equal(t, exec.Name, util.CommandJobName(exec))
	cronName := util.CommandCronJobName(exec)
	assertValidName(t, "command cronjob", cronName, validation.IsDNS1123Subdomain)
	assertMaxLen(t, "command cronjob", cronName, util.MaxCronJobNameLength)

	short := v1.StoreExec{ObjectMeta: metav1.ObjectMeta{Name: "cache.clear"}}
	assert.Equal(t, "cache.clear", util.CommandJobName(short))
	assert.Equal(t, "cache.clear", util.CommandCronJobName(short))
}

func TestSnapshotJobName(t *testing.T) {
	for _, subCommand := range []string{util.SnapshotSubCommandCreate, util.SnapshotSubCommandRestore} {
		name := util.SnapshotJobName(metav1.ObjectMeta{Name: strings.Repeat("a", 63)}, subCommand)
		assertValidName(t, "snapshot job", name, validation.IsDNS1123Subdomain)
		assertMaxLen(t, "snapshot job", name, util.MaxNameLength)
	}

	assert.Equal(t, "snapshot-create-backup.daily",
		util.SnapshotJobName(metav1.ObjectMeta{Name: "backup.daily"}, util.SnapshotSubCommandCreate))
}

func TestDebugNames(t *testing.T) {
	instance := v1.StoreDebugInstance{ObjectMeta: metav1.ObjectMeta{Name: "debug-1"}}

	assert.Equal(t, "debug-1", util.GetDebugPodName(instance))
	assert.Equal(t, "debug-1", util.GetDebugServiceName(instance))
}
