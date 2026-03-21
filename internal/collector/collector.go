// Package collector implements Prometheus collectors for Nomad state objects.
package collector

import (
	"log/slog"
	"sync"

	nomadapi "github.com/hashicorp/nomad/api"
	"github.com/prometheus/client_golang/prometheus"
)

// Collector gathers metrics from the Nomad API and implements prometheus.Collector.
type Collector struct {
	client *nomadapi.Client
	logger *slog.Logger

	// Per-resource sub-collectors
	sub []prometheus.Collector
}

// New returns a new Collector that scrapes the given Nomad client.
func New(client *nomadapi.Client, logger *slog.Logger) *Collector {
	c := &Collector{
		client: client,
		logger: logger,
	}
	c.sub = []prometheus.Collector{
		newJobCollector(client, logger),
		newNodeCollector(client, logger),
		newAllocationCollector(client, logger),
		newDeploymentCollector(client, logger),
		newEvaluationCollector(client, logger),
	}
	return c
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
