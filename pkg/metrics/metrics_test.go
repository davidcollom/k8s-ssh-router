package metrics

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"testing"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// freePort reserves an ephemeral port and releases it again so that the
// metrics server can bind to it.
func freePort(t *testing.T) int {
	t.Helper()
	l, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err, "Failed to reserve a port")
	defer l.Close()
	return l.Addr().(*net.TCPAddr).Port
}

func TestStartMetricsServer(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	port := freePort(t)
	go func() {
		_ = StartMetricsServer(ctx, port)
	}()

	url := fmt.Sprintf("http://127.0.0.1:%d/metrics", port)
	var resp *http.Response
	var err error

	// The server starts asynchronously, so wait for it to accept connections
	// instead of racing it.
	deadline := time.Now().Add(10 * time.Second)
	for time.Now().Before(deadline) {
		var req *http.Request
		req, err = http.NewRequest("GET", url, nil)
		require.NoError(t, err, "Failed to create request")

		resp, err = http.DefaultClient.Do(req)
		if err == nil {
			break
		}
		time.Sleep(50 * time.Millisecond)
	}
	require.NoError(t, err, "Failed to get response")
	require.NotNil(t, resp, "Expected a response")
	assert.Equal(t, http.StatusOK, resp.StatusCode, "Expected status 200")
}

func TestStartMetricsServerStopsOnCancel(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	port := freePort(t)

	stopped := make(chan error, 1)
	go func() {
		stopped <- StartMetricsServer(ctx, port)
	}()

	// Wait for the server to accept connections before shutting it down.
	url := fmt.Sprintf("http://127.0.0.1:%d/metrics", port)
	deadline := time.Now().Add(10 * time.Second)
	for time.Now().Before(deadline) {
		resp, err := http.Get(url)
		if err == nil {
			resp.Body.Close()
			break
		}
		time.Sleep(50 * time.Millisecond)
	}

	cancel()

	select {
	case err := <-stopped:
		require.NoError(t, err, "Expected a clean shutdown")
	case <-time.After(15 * time.Second):
		t.Fatal("Metrics server did not shut down after the context was cancelled")
	}
}

func TestActiveSessions(t *testing.T) {
	IncActiveSessions()
	IncActiveSessions()
	gatherers := prometheus.Gatherers{prometheus.DefaultGatherer}
	metrics, err := gatherers.Gather()
	assert.NoError(t, err, "Expected no error gathering metrics")

	var found bool
	for _, m := range metrics {
		if m.GetName() == "active_ssh_sessions" {
			assert.Equal(t, 2.0, *m.Metric[0].Gauge.Value, "Expected 2 active sessions")
			found = true
			break
		}
	}
	assert.True(t, found, "Expected active_ssh_sessions metric to be found")

	DecActiveSessions()
}
