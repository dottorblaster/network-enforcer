package cniwatcher_test

import (
	"testing"

	"github.com/rancher-sandbox/network-enforcer/internal/cniwatcher"
	"github.com/rancher-sandbox/network-enforcer/internal/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestParseCNIType(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected types.CNIType
		wantErr  bool
	}{
		{
			name:     "Valid AWS VPC CNI type",
			input:    string(types.CNITypeAWSVPC),
			expected: types.CNITypeAWSVPC,
			wantErr:  false,
		},
		{
			name:     "Valid Calico CNI type",
			input:    string(types.CNITypeCalico),
			expected: types.CNITypeCalico,
			wantErr:  false,
		},
		{
			name:     "Valid Cilium CNI type",
			input:    string(types.CNITypeCilium),
			expected: types.CNITypeCilium,
			wantErr:  false,
		},
		{
			name:     "Valid Flannel CNI type",
			input:    string(types.CNITypeFlannel),
			expected: types.CNITypeFlannel,
			wantErr:  false,
		},
		{
			name:     "Empty CNI type",
			input:    "",
			expected: types.CNITypeUnknown,
			wantErr:  true,
		},
		{
			name:     "Invalid CNI type",
			input:    "invalid-cni",
			expected: types.CNITypeUnknown,
			wantErr:  true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			config, err := cniwatcher.NewConfig("test-node", tt.input)
			if tt.wantErr {
				require.Error(t, err)
			} else {
				require.NoError(t, err)
				assert.Equal(t, tt.expected, config.CNIType)
			}
		})
	}
}

func TestNewConfig(t *testing.T) {
	tests := []struct {
		name             string
		nodeName         string
		cniType          string
		connEndpoint     string
		wantErr          bool
		expectedCNI      types.CNIType
		expectedEndpoint string
	}{
		{
			name:             "Valid AWS VPC config",
			nodeName:         "test-node",
			cniType:          string(types.CNITypeAWSVPC),
			wantErr:          false,
			expectedCNI:      types.CNITypeAWSVPC,
			expectedEndpoint: "",
		},
		{
			name:             "Valid Flannel config",
			nodeName:         "test-node",
			cniType:          string(types.CNITypeFlannel),
			wantErr:          false,
			expectedCNI:      types.CNITypeFlannel,
			expectedEndpoint: "",
		},
		{
			name:             "Valid Calico config with default endpoint",
			nodeName:         "test-node",
			cniType:          string(types.CNITypeCalico),
			connEndpoint:     types.DefaultGoldmaneEndpoint,
			wantErr:          false,
			expectedCNI:      types.CNITypeCalico,
			expectedEndpoint: types.DefaultGoldmaneEndpoint,
		},
		{
			name:             "Valid Calico config with empty endpoint",
			nodeName:         "test-node",
			cniType:          string(types.CNITypeCalico),
			connEndpoint:     "",
			wantErr:          false,
			expectedCNI:      types.CNITypeCalico,
			expectedEndpoint: types.DefaultGoldmaneEndpoint,
		},
		{
			name:             "Valid Cilium config with default endpoint",
			nodeName:         "test-node",
			cniType:          string(types.CNITypeCilium),
			connEndpoint:     types.DefaultHubbleEndpoint,
			wantErr:          false,
			expectedCNI:      types.CNITypeCilium,
			expectedEndpoint: types.DefaultHubbleEndpoint,
		},
		{
			name:             "Valid Cilium config with empty endpoint",
			nodeName:         "test-node",
			cniType:          string(types.CNITypeCilium),
			connEndpoint:     "",
			wantErr:          false,
			expectedCNI:      types.CNITypeCilium,
			expectedEndpoint: types.DefaultHubbleEndpoint,
		},
		{
			name:             "Empty node name",
			nodeName:         "",
			cniType:          string(types.CNITypeCalico),
			wantErr:          true,
			expectedCNI:      types.CNITypeUnknown,
			expectedEndpoint: "",
		},
		{
			name:             "Unknown CNI type",
			nodeName:         "test-node",
			cniType:          string(types.CNITypeUnknown),
			wantErr:          true,
			expectedCNI:      types.CNITypeUnknown,
			expectedEndpoint: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			config, err := cniwatcher.NewConfig(tt.nodeName, tt.cniType)
			if tt.wantErr {
				require.Error(t, err)
				assert.Equal(t, cniwatcher.Config{}, config)
			} else {
				require.NoError(t, err)
				assert.Equal(t, tt.nodeName, config.NodeName)
				assert.Equal(t, tt.expectedCNI, config.CNIType)
				assert.Equal(t, tt.expectedEndpoint, config.ConnEndpoint)
			}
		})
	}
}
