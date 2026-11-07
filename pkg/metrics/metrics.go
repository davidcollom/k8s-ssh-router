package metrics

import (
	"context"
	"errors"
	"fmt"
	"log"
	"net/http"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

// shutdownTimeout is how long the metrics server is given to finish the
// requests that are in flight when a shutdown is requested.
const shutdownTimeout = 10 * time.Second

var activeSessions = prometheus.NewGauge(prometheus.GaugeOpts{
	Name: "active_ssh_sessions",
	Help: "Number of active SSH sessions",
})

func init() {
	prometheus.MustRegister(activeSessions)
}

// MetricsHandler returns the handler that serves the Prometheus metrics.
func MetricsHandler() http.Handler {
	mux := http.NewServeMux()
	mux.Handle("/metrics", promhttp.Handler())
	return mux
}

// StartMetricsServer serves the Prometheus metrics endpoint on the given port.
// It blocks until the server stops serving or the context is cancelled.
func StartMetricsServer(ctx context.Context, port int) error {
	server := &http.Server{
		Addr:              fmt.Sprintf(":%d", port),
		Handler:           MetricsHandler(),
		ReadHeaderTimeout: 10 * time.Second,
	}

	go shutdownServerOnCancel(ctx, server)

	if err := server.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
		return err
	}
	return nil
}

// shutdownServerOnCancel gracefully shuts the server down when the context is
// cancelled.
func shutdownServerOnCancel(ctx context.Context, server *http.Server) {
	<-ctx.Done()

	shutdownCtx, cancel := context.WithTimeout(context.Background(), shutdownTimeout)
	defer cancel()

	if err := server.Shutdown(shutdownCtx); err != nil {
		log.Printf("Metrics server shutdown failed: %v", err)
	}
}

func IncActiveSessions() {
	activeSessions.Inc()
}

func DecActiveSessions() {
	activeSessions.Dec()
}
