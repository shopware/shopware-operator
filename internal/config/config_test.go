package config

import (
	"context"
	"os"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestStoreConfigEnablesWebhookByDefault(t *testing.T) {
	previousValue, wasSet := os.LookupEnv("ENABLE_WEBHOOK")
	require.NoError(t, os.Unsetenv("ENABLE_WEBHOOK"))
	t.Cleanup(func() {
		if wasSet {
			require.NoError(t, os.Setenv("ENABLE_WEBHOOK", previousValue))
			return
		}
		require.NoError(t, os.Unsetenv("ENABLE_WEBHOOK"))
	})

	cfg, err := LoadStoreConfig(context.Background())
	require.NoError(t, err)
	require.True(t, cfg.EnableWebhook)
}

func TestStoreConfigCanDisableWebhook(t *testing.T) {
	t.Setenv("ENABLE_WEBHOOK", "false")

	cfg, err := LoadStoreConfig(context.Background())
	require.NoError(t, err)
	require.False(t, cfg.EnableWebhook)
}
