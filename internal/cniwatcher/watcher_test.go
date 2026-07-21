package cniwatcher_test

import (
	"log/slog"
	"os"
	"testing"

	"github.com/rancher-sandbox/network-enforcer/internal/cniwatcher"
	"github.com/rancher-sandbox/network-enforcer/internal/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

func testLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelError}))
}

func TestNewCNIWatcher(t *testing.T) {
	tests := []struct {
		name    string
		config  cniwatcher.Config
		wantErr bool
	}{
		{
			name: "Valid Calico config",
			config: cniwatcher.Config{
				NodeName:     "test-node",
				CNIType:      types.CNITypeCalico,
				ConnEndpoint: types.DefaultGoldmaneEndpoint,
			},
			wantErr: false,
		},
		{
			name: "Valid Cilium config",
			config: cniwatcher.Config{
				NodeName:     "test-node",
				CNIType:      types.CNITypeCilium,
				ConnEndpoint: types.DefaultHubbleEndpoint,
			},
			wantErr: false,
		},
		{
			name: "Valid AWS VPC config",
			config: cniwatcher.Config{
				NodeName: "test-node",
				CNIType:  types.CNITypeAWSVPC,
			},
			wantErr: false,
		},
		{
			name: "Valid Flannel config",
			config: cniwatcher.Config{
				NodeName: "test-node",
				CNIType:  types.CNITypeFlannel,
			},
			wantErr: false,
		},
		{
			name: "Unknown CNI type",
			config: cniwatcher.Config{
				NodeName: "test-node",
				CNIType:  types.CNITypeUnknown,
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			watcher := cniwatcher.Watcher{
				Ctx:    t.Context(),
				Client: fake.NewClientBuilder().Build(),
				Log:    testLogger(),
			}

			cniWatcher, err := cniwatcher.NewCNIWatcher(tt.config, watcher)
			if tt.wantErr {
				require.Error(t, err)
				assert.Nil(t, cniWatcher)
			} else {
				require.NoError(t, err)
				assert.NotNil(t, cniWatcher)
			}
		})
	}
}

func TestWatcher_GetNetworkPolicyAPIVersion(t *testing.T) {
	watcher := &cniwatcher.Watcher{Log: testLogger()}

	tests := []struct {
		name    string
		kind    string
		want    string
		wantErr bool
	}{
		{
			name:    "NetworkPolicy",
			kind:    "NetworkPolicy",
			want:    "networking.k8s.io/v1",
			wantErr: false,
		},
		{
			name:    "CalicoNetworkPolicy",
			kind:    "CalicoNetworkPolicy",
			want:    "projectcalico.org/v3",
			wantErr: false,
		},
		{
			name:    "GlobalNetworkPolicy",
			kind:    "GlobalNetworkPolicy",
			want:    "projectcalico.org/v3",
			wantErr: false,
		},
		{
			name:    "CiliumNetworkPolicy",
			kind:    "CiliumNetworkPolicy",
			want:    "cilium.io/v2",
			wantErr: false,
		},
		{
			name:    "CiliumClusterwideNetworkPolicy",
			kind:    "CiliumClusterwideNetworkPolicy",
			want:    "cilium.io/v2",
			wantErr: false,
		},
		{
			name:    "Unknown policy kind",
			kind:    "UnknownPolicy",
			want:    "",
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := watcher.GetNetworkPolicyAPIVersion(tt.kind)
			if tt.wantErr {
				assert.Error(t, err)
			} else {
				require.NoError(t, err)
				assert.Equal(t, tt.want, got)
			}
		})
	}
}

func TestWatcher_ResolvePodOrServiceByIP(t *testing.T) {
	tests := []struct {
		name     string
		ip       string
		pods     []*corev1.Pod
		services []*corev1.Service
		wantName string
		wantNS   string
		wantErr  bool
	}{
		{
			name: "Pod by IP",
			ip:   "10.0.0.1",
			pods: []*corev1.Pod{{
				ObjectMeta: metav1.ObjectMeta{Name: "test-pod", Namespace: "default"},
				Status:     corev1.PodStatus{PodIP: "10.0.0.1"},
			}},
			wantName: "test-pod",
			wantNS:   "default",
		},
		{
			name: "Service by cluster IP",
			ip:   "10.0.0.10",
			services: []*corev1.Service{{
				ObjectMeta: metav1.ObjectMeta{Name: "test-svc", Namespace: "default"},
				Spec:       corev1.ServiceSpec{ClusterIP: "10.0.0.10"},
			}},
			wantName: "test-svc",
			wantNS:   "default",
		},
		{
			name: "Service by external IP",
			ip:   "203.0.113.10",
			services: []*corev1.Service{{
				ObjectMeta: metav1.ObjectMeta{Name: "ext-svc", Namespace: "default"},
				Spec: corev1.ServiceSpec{
					ClusterIP:   "10.0.0.20",
					ExternalIPs: []string{"203.0.113.10"},
				},
			}},
			wantName: "ext-svc",
			wantNS:   "default",
		},
		{
			name:    "Invalid IP",
			ip:      "invalid-ip",
			wantErr: true,
		},
		{
			name:    "Empty IP",
			ip:      "",
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			builder := fake.NewClientBuilder().
				WithIndex(&corev1.Pod{}, "status.podIP", func(obj client.Object) []string {
					pod := obj.(*corev1.Pod)
					if pod.Status.PodIP == "" {
						return nil
					}
					return []string{pod.Status.PodIP}
				}).
				WithIndex(&corev1.Service{}, "spec.clusterIP", func(obj client.Object) []string {
					svc := obj.(*corev1.Service)
					if svc.Spec.ClusterIP == "" || svc.Spec.ClusterIP == "None" {
						return nil
					}
					return []string{svc.Spec.ClusterIP}
				})

			for _, pod := range tt.pods {
				builder = builder.WithObjects(pod)
			}
			for _, svc := range tt.services {
				builder = builder.WithObjects(svc)
			}

			watcher := &cniwatcher.Watcher{
				Ctx:    t.Context(),
				Client: builder.Build(),
				Log:    testLogger(),
			}

			info, err := watcher.ResolvePodOrServiceByIP(tt.ip)
			if tt.wantErr {
				require.Error(t, err)
				assert.Empty(t, info.Name)
				assert.Empty(t, info.Namespace)
			} else {
				require.NoError(t, err)
				assert.Equal(t, tt.wantName, info.Name)
				assert.Equal(t, tt.wantNS, info.Namespace)
			}
		})
	}
}
