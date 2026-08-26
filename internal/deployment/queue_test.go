package deployment

import (
	"testing"

	v1 "github.com/shopware/shopware-operator/api/v1"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestParseMessengerStats(t *testing.T) {
	tests := []struct {
		name                string
		input               string
		expected            []v1.QueueTransportStats
		expectedUncountable []string
		wantErr             bool
	}{
		{
			name:  "wrapped transports list",
			input: `{"transports":[{"name":"async","count":42},{"name":"failed","count":3}]}`,
			expected: []v1.QueueTransportStats{
				{Name: "async", Count: 42},
				{Name: "failed", Count: 3},
			},
		},
		{
			name:  "list with transport key",
			input: `[{"transport":"async","count":7}]`,
			expected: []v1.QueueTransportStats{
				{Name: "async", Count: 7},
			},
		},
		{
			name:  "wrapped transports map",
			input: `{"transports":{"low_priority":0,"async":5}}`,
			expected: []v1.QueueTransportStats{
				{Name: "async", Count: 5},
				{Name: "low_priority", Count: 0},
			},
		},
		{
			name: "shopware 6.7 real output with uncountable transports",
			input: `{
				"transports": {
					"failed": {"count": 3},
					"async": {"count": 42},
					"low_priority": {"count": 0},
					"mail": {"count": 1}
				},
				"uncountable_transports": ["webhook", "scheduler_shopware"]
			}`,
			expected: []v1.QueueTransportStats{
				{Name: "async", Count: 42},
				{Name: "failed", Count: 3},
				{Name: "low_priority", Count: 0},
				{Name: "mail", Count: 1},
			},
			expectedUncountable: []string{"webhook", "scheduler_shopware"},
		},
		{
			name:  "plain map",
			input: `{"async":1,"failed":2}`,
			expected: []v1.QueueTransportStats{
				{Name: "async", Count: 1},
				{Name: "failed", Count: 2},
			},
		},
		{
			name:    "empty output",
			input:   "  ",
			wantErr: true,
		},
		{
			name:    "garbage output",
			input:   "PHP Fatal error: something broke",
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			stats, uncountable, err := parseMessengerStats([]byte(tt.input))
			if tt.wantErr {
				require.Error(t, err)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tt.expected, stats)
			assert.Equal(t, tt.expectedUncountable, uncountable)
		})
	}
}
