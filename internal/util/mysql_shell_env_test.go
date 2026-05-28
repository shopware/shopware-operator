package util

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestBuildMySQLShellEnvOverridesHomeAndConfig(t *testing.T) {
	env := buildMySQLShellEnv([]string{
		"PATH=/usr/bin",
		"HOME=/",
		mysqlShellConfigHomeEnvKey + "=/tmp/original-config",
	}, "/tmp/mysqlsh-home", "/tmp/mysqlsh-home/.config")

	assert.Contains(t, env, "PATH=/usr/bin")
	assert.Contains(t, env, "HOME=/tmp/mysqlsh-home")
	assert.Contains(t, env, mysqlShellConfigHomeEnvKey+"=/tmp/mysqlsh-home/.config")
	assert.NotContains(t, env, "HOME=/")
	assert.NotContains(t, env, mysqlShellConfigHomeEnvKey+"=/tmp/original-config")
	assert.Len(t, env, 3)

	homeCount := 0
	configCount := 0
	for _, entry := range env {
		switch entry {
		case "HOME=/tmp/mysqlsh-home":
			homeCount++
		case mysqlShellConfigHomeEnvKey + "=/tmp/mysqlsh-home/.config":
			configCount++
		}
	}

	require.Equal(t, 1, homeCount)
	require.Equal(t, 1, configCount)
}
