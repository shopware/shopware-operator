package util_test

import (
	"strings"
	"testing"

	v1 "github.com/shopware/shopware-operator/api/v1"
	"github.com/shopware/shopware-operator/internal/util"
	"github.com/stretchr/testify/assert"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func storeWithName(name string) v1.Store {
	return v1.Store{ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: "test"}}
}

func TestValidateStoreNames(t *testing.T) {
	t.Run("valid store", func(t *testing.T) {
		store := storeWithName("my-store")
		store.Status.QueueState.Transports = []v1.QueueTransportStats{{Name: "async"}, {Name: "low_priority"}}
		assert.NoError(t, util.ValidateStoreNames(store))
	})

	t.Run("max length store name with long queue", func(t *testing.T) {
		store := storeWithName(strings.Repeat("a", 63))
		store.Status.QueueState.Transports = []v1.QueueTransportStats{{Name: strings.Repeat("q", 80)}}
		assert.NoError(t, util.ValidateStoreNames(store))
	})

	t.Run("too long store name", func(t *testing.T) {
		err := util.ValidateStoreNames(storeWithName(strings.Repeat("a", 64)))
		assert.ErrorContains(t, err, "store name label")
	})

	t.Run("store name starting with digit breaks service name", func(t *testing.T) {
		err := util.ValidateStoreNames(storeWithName("1shop"))
		assert.ErrorContains(t, err, "admin service")
		assert.ErrorContains(t, err, "storefront service")
	})
}

func TestValidateStoreExecNames(t *testing.T) {
	assert.NoError(t, util.ValidateStoreExecNames(v1.StoreExec{ObjectMeta: metav1.ObjectMeta{Name: "cache-clear"}}))
	assert.Error(t, util.ValidateStoreExecNames(v1.StoreExec{ObjectMeta: metav1.ObjectMeta{Name: strings.Repeat("a", 64)}}))
}

func TestValidateSnapshotNames(t *testing.T) {
	assert.NoError(t, util.ValidateSnapshotNames(strings.Repeat("a", 63), util.SnapshotSubCommandRestore))
	assert.Error(t, util.ValidateSnapshotNames(strings.Repeat("a", 64), util.SnapshotSubCommandCreate))
}

func TestValidateDebugInstanceNames(t *testing.T) {
	assert.NoError(t, util.ValidateDebugInstanceNames(v1.StoreDebugInstance{ObjectMeta: metav1.ObjectMeta{Name: "debug-1"}}))
	assert.ErrorContains(t,
		util.ValidateDebugInstanceNames(v1.StoreDebugInstance{ObjectMeta: metav1.ObjectMeta{Name: "1-debug"}}),
		"debug service")
}

func TestShortName(t *testing.T) {
	t.Run("short name is unchanged", func(t *testing.T) {
		assert.Equal(t, "my-store-admin", util.ShortName("my-store-admin"))
	})

	t.Run("name at limit is unchanged", func(t *testing.T) {
		name := strings.Repeat("a", util.MaxNameLength)
		assert.Equal(t, name, util.ShortName(name))
	})

	t.Run("long name is truncated with hash", func(t *testing.T) {
		name := strings.Repeat("a", 80) + "-store-admin"
		short := util.ShortName(name)
		assert.Len(t, short, util.MaxNameLength)
		assert.Equal(t, short, util.ShortName(name))
		assert.NotEqual(t, short, util.ShortName(strings.Repeat("a", 80)+"-store-worker"))
	})

	t.Run("no trailing dash before hash", func(t *testing.T) {
		name := strings.Repeat("a", 53) + "----------" + "b"
		short := util.ShortName(name)
		assert.LessOrEqual(t, len(short), util.MaxNameLength)
		assert.NotContains(t, short, "--")
	})

	t.Run("cronjob limit", func(t *testing.T) {
		short := util.ShortNameWithLimit(strings.Repeat("a", 60)+"-scheduled-jobs", util.MaxCronJobNameLength)
		assert.Len(t, short, util.MaxCronJobNameLength)
	})
}
