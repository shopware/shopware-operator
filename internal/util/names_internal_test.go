package util

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestShortNameWithLimit(t *testing.T) {
	t.Run("short name is unchanged", func(t *testing.T) {
		assert.Equal(t, "my-store-admin", shortNameWithLimit("my-store-admin", MaxNameLength))
	})

	t.Run("name at limit is unchanged", func(t *testing.T) {
		name := strings.Repeat("a", MaxNameLength)
		assert.Equal(t, name, shortNameWithLimit(name, MaxNameLength))
	})

	t.Run("long name is truncated with hash", func(t *testing.T) {
		name := strings.Repeat("a", 80) + "-store-admin"
		short := shortNameWithLimit(name, MaxNameLength)
		assert.Len(t, short, MaxNameLength)
		assert.Equal(t, short, shortNameWithLimit(name, MaxNameLength))
		assert.NotEqual(t, short, shortNameWithLimit(strings.Repeat("a", 80)+"-store-worker", MaxNameLength))
	})

	t.Run("no trailing dash before hash", func(t *testing.T) {
		name := strings.Repeat("a", 53) + "----------" + "b"
		short := shortNameWithLimit(name, MaxNameLength)
		assert.LessOrEqual(t, len(short), MaxNameLength)
		assert.NotContains(t, short, "--")
	})

	t.Run("cronjob limit", func(t *testing.T) {
		short := shortNameWithLimit(strings.Repeat("a", 60)+"-scheduled-jobs", MaxCronJobNameLength)
		assert.Len(t, short, MaxCronJobNameLength)
	})
}
