package cniwatcher_test

import (
	"log/slog"
	"os"
	"testing"

	"github.com/rancher-sandbox/network-enforcer/internal/cniwatcher"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

func TestNewAWSVPCWatcher(t *testing.T) {
	fakeClient := fake.NewClientBuilder().Build()
	log := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelError}))

	watcher, err := cniwatcher.NewAWSVPCWatcher(cniwatcher.Watcher{
		Ctx:    t.Context(),
		Client: fakeClient,
		Log:    log,
	})
	require.NoError(t, err)
	assert.NotNil(t, watcher)
}

func TestAWSVPCWatcher_Shutdown(t *testing.T) {
	watcher := &cniwatcher.AWSVPCWatcher{
		Watcher: cniwatcher.Watcher{
			Log: slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelError})),
		},
	}
	assert.NoError(t, watcher.Shutdown())
}
