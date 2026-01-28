package secret

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestDataChecksum(t *testing.T) {
	base := map[string][]byte{
		"database-url": []byte("mysql://user:pass@host/db"),
		"app-secret":   []byte("secret"),
	}

	tests := []struct {
		name    string
		data    map[string][]byte
		changed bool
	}{
		{
			name:    "same data",
			data:    map[string][]byte{"app-secret": []byte("secret"), "database-url": []byte("mysql://user:pass@host/db")},
			changed: false,
		},
		{
			name:    "key added",
			data:    map[string][]byte{"app-secret": []byte("secret"), "database-url": []byte("mysql://user:pass@host/db"), "random": []byte("value")},
			changed: true,
		},
		{
			name:    "value changed",
			data:    map[string][]byte{"app-secret": []byte("other"), "database-url": []byte("mysql://user:pass@host/db")},
			changed: true,
		},
		{
			name:    "key removed",
			data:    map[string][]byte{"app-secret": []byte("secret")},
			changed: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.changed {
				require.NotEqual(t, dataChecksum(base), dataChecksum(tt.data))
			} else {
				require.Equal(t, dataChecksum(base), dataChecksum(tt.data))
			}
		})
	}
}
