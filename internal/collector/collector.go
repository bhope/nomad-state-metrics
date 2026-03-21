// Package collector implements Prometheus collectors for Nomad state objects.
package collector

import (
	"log/slog"
	"sync"

	"github.com/prometheus/client_golang/prometheus"

	"github.com/bhope/nomad-state-metrics/internal/store"
)

// Collector gathers metrics from a NomadStore and implements prometheus.Collector.
type Collector struct {
	sub []prometheus.Collector
}

// New returns a new Collector that reads from the given NomadStore.
func New(s *store.NomadStore, logger *slog.Logger) *Collector {
	return &Collector{
		sub: []prometheus.Collector{
			newJobCollector(s, logger),
			newNodeCollector(s, logger),
			newAllocationCollector(s, logger),
			newDeploymentCollector(s, logger),
			newEvaluationCollector(s, logger),
		},
	}
}

// Describe implements prometheus.Collector.
func (c *Collector) Describe(ch chan<- *prometheus.Desc) {
	for _, s := range c.sub {
		s.Describe(ch)
	}
}

// Collect implements prometheus.Collector.
func (c *Collector) Collect(ch chan<- prometheus.Metric) {
	var wg sync.WaitGroup
	for _, s := range c.sub {
		wg.Add(1)
		go func(s prometheus.Collector) {
			defer wg.Done()
			s.Collect(ch)
		}(s)
	}
	wg.Wait()
}
