package main

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestParseTempSize(t *testing.T) {
	bytes, err := parseTempSize("100Gi")
	require.NoError(t, err)
	assert.Equal(t, int64(100*1024*1024*1024), bytes)

	bytes, err = parseTempSize("1.5Gi")
	require.NoError(t, err)
	assert.Equal(t, int64(1610612736), bytes)

	_, err = parseTempSize("0")
	require.Error(t, err)

	_, err = parseTempSize("-1Gi")
	require.Error(t, err)
}
