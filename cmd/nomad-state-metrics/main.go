// Package main is the entry point for the nomad-state-metrics server.
package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	nomadapi "github.com/hashicorp/nomad/api"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/collectors"
	"github.com/prometheus/client_golang/prometheus/promhttp"

	"github.com/bhope/nomad-state-metrics/internal/collector"
	"github.com/bhope/nomad-state-metrics/internal/store"
)

func main() {
	var (
		nomadAddr       = flag.String("nomad-address", "http://localhost:4646", "Nomad API address (ignored when -task-api is set)")
		taskAPI         = flag.Bool("task-api", false, "Use Nomad Task API via Unix Domain Socket (reads NOMAD_TASK_API_UDS env var)")
		jobStatusFilter = flag.String("job-status-filter", "", "Comma-separated job statuses to include (e.g. running,pending). Empty includes all.")
		port            = flag.Int("port", 9441, "Port to serve Nomad state metrics on")
		telemetryPort   = flag.Int("telemetry-port", 9442, "Port to serve self-metrics and /healthz on")
		pollInterval    = flag.Duration("poll-interval", 30*time.Second, "Interval between Nomad API polls")
		logLevel        = flag.String("log-level", "info", "Log level (debug, info, warn, error)")
	)
	flag.Parse()

	var level slog.Level
	if err := level.UnmarshalText([]byte(*logLevel)); err != nil {
		slog.Error("invalid log level", "level", *logLevel)
		os.Exit(1)
	}
	logger := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: level}))
	slog.SetDefault(logger)

	cfg := nomadapi.DefaultConfig()
	if *taskAPI {
		udsPath := os.Getenv("NOMAD_TASK_API_UDS")
		if udsPath == "" {
			slog.Error("NOMAD_TASK_API_UDS is not set; required when -task-api is enabled")
			os.Exit(1)
		}
		cfg.Address = "unix://" + udsPath
		slog.Info("using Nomad Task API via Unix Domain Socket", "socket", udsPath)
	} else {
		cfg.Address = *nomadAddr
	}

	client, err := nomadapi.NewClient(cfg)
	if err != nil {
		slog.Error("failed to create Nomad client", "error", err)
		os.Exit(1)
	}

	var statusFilter map[string]struct{}
	if *jobStatusFilter != "" {
		statusFilter = make(map[string]struct{})
		for _, s := range strings.Split(*jobStatusFilter, ",") {
			statusFilter[strings.TrimSpace(s)] = struct{}{}
		}
	}

	nomadStore := store.New(client, *pollInterval, logger, statusFilter)

	// Main registry: Nomad state metrics only.
	stateRegistry := prometheus.NewRegistry()
	stateRegistry.MustRegister(collector.New(nomadStore, logger))

	// Telemetry registry: Go runtime and process metrics.
	telemetryRegistry := prometheus.NewRegistry()
	telemetryRegistry.MustRegister(
		collectors.NewGoCollector(),
		collectors.NewProcessCollector(collectors.ProcessCollectorOpts{}),
	)

	stateMux := http.NewServeMux()
	stateMux.Handle("/metrics", promhttp.HandlerFor(stateRegistry, promhttp.HandlerOpts{
		EnableOpenMetrics: true,
	}))

	telemetryMux := http.NewServeMux()
	telemetryMux.Handle("/metrics", promhttp.HandlerFor(telemetryRegistry, promhttp.HandlerOpts{
		EnableOpenMetrics: true,
	}))
	telemetryMux.HandleFunc("/healthz", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	stateServer := &http.Server{
		Addr:    fmt.Sprintf(":%d", *port),
		Handler: stateMux,
	}
	telemetryServer := &http.Server{
		Addr:    fmt.Sprintf(":%d", *telemetryPort),
		Handler: telemetryMux,
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Start polling the Nomad API in the background.
	go nomadStore.Run(ctx)

	// Start both HTTP servers.
	go func() {
		slog.Info("serving Nomad state metrics", "port", *port)
		if err := stateServer.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			slog.Error("state metrics server exited", "error", err)
			cancel()
		}
	}()
	go func() {
		slog.Info("serving telemetry and healthz", "port", *telemetryPort)
		if err := telemetryServer.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			slog.Error("telemetry server exited", "error", err)
			cancel()
		}
	}()

	// Block until a signal or a server error cancels the context.
	sig := make(chan os.Signal, 1)
	signal.Notify(sig, syscall.SIGTERM, syscall.SIGINT)
	select {
	case s := <-sig:
		slog.Info("received signal, shutting down", "signal", s)
	case <-ctx.Done():
	}

	cancel()

	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer shutdownCancel()

	_ = stateServer.Shutdown(shutdownCtx)
	_ = telemetryServer.Shutdown(shutdownCtx)

	slog.Info("shutdown complete")
}
