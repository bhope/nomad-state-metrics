package main

import (
	"flag"
	"log/slog"
	"net/http"
	"os"

	nomadapi "github.com/hashicorp/nomad/api"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"

	"github.com/bhope/nomad-state-metrics/internal/collector"
)

func main() {
	var (
		listenAddr  = flag.String("listen-address", ":9290", "Address to listen on for HTTP requests")
		nomadAddr   = flag.String("nomad-address", "", "Nomad API address (overrides NOMAD_ADDR)")
		nomadToken  = flag.String("nomad-token", "", "Nomad ACL token (overrides NOMAD_TOKEN)")
		logLevel    = flag.String("log-level", "info", "Log level (debug, info, warn, error)")
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
	if *nomadAddr != "" {
		cfg.Address = *nomadAddr
	}
	if *nomadToken != "" {
		cfg.SecretID = *nomadToken
	}

	client, err := nomadapi.NewClient(cfg)
	if err != nil {
		slog.Error("failed to create Nomad client", "error", err)
		os.Exit(1)
	}

	registry := prometheus.NewRegistry()
	registry.MustRegister(collector.New(client, logger))

	mux := http.NewServeMux()
	mux.Handle("/metrics", promhttp.HandlerFor(registry, promhttp.HandlerOpts{
		EnableOpenMetrics: true,
	}))
	mux.HandleFunc("/healthz", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	slog.Info("starting nomad-state-metrics", "listen", *listenAddr)
	if err := http.ListenAndServe(*listenAddr, mux); err != nil {
		slog.Error("server exited", "error", err)
		os.Exit(1)
	}
}
