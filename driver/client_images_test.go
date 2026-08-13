package driver

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestNetworkConfig_GetClientImages(t *testing.T) {
	tests := map[string]struct {
		config NetworkConfig
		want   []string
	}{
		"empty network": {
			config: NetworkConfig{},
			want:   []string{},
		},
		"validator without image uses the default": {
			config: NetworkConfig{Validators: Validators{{Name: "validator"}}},
			want:   []string{DefaultClientDockerImageName},
		},
		"validators and later nodes": {
			config: NetworkConfig{
				Validators:   Validators{{Name: "validator", ImageName: "sonic:v2.2.0"}},
				ClientImages: []string{"sonic:local", "sonic:v2.1.6"},
			},
			want: []string{"sonic:v2.2.0", "sonic:local", "sonic:v2.1.6"},
		},
	}

	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			require.Equal(t, test.want, test.config.GetClientImages())
		})
	}
}
