package collector

import (
	"log/slog"

	"github.com/prometheus/client_golang/prometheus"

	"github.com/bhope/nomad-state-metrics/internal/store"
)

var (
	allocInfo = prometheus.NewDesc(
		"nomad_allocation_info",
		"Information about a Nomad allocation.",
		[]string{"alloc_id", "alloc_name", "job", "task_group", "namespace", "node_id", "client_status", "desired_status"},
		nil,
	)
	allocsTotal = prometheus.NewDesc(
		"nomad_allocations_total",
		"Total number of Nomad allocations by client and desired status.",
		[]string{"namespace", "client_status", "desired_status"},
		nil,
	)
)

type allocationCollector struct {
	store  *store.NomadStore
	logger *slog.Logger
}

func newAllocationCollector(s *store.NomadStore, logger *slog.Logger) *allocationCollector {
	return &allocationCollector{store: s, logger: logger}
}

func (c *allocationCollector) Describe(ch chan<- *prometheus.Desc) {
	ch <- allocInfo
	ch <- allocsTotal
}

func (c *allocationCollector) Collect(ch chan<- prometheus.Metric) {
	allocs := c.store.ListAllocations()

	type key struct{ namespace, clientStatus, desiredStatus string }
	counts := map[key]int{}

	for _, a := range allocs {
		ch <- prometheus.MustNewConstMetric(
			allocInfo, prometheus.GaugeValue, 1,
			a.ID, a.Name, a.JobID, a.TaskGroup, a.Namespace, a.NodeID,
			a.ClientStatus, a.DesiredStatus,
		)
		counts[key{a.Namespace, a.ClientStatus, a.DesiredStatus}]++
	}

	for k, cnt := range counts {
		ch <- prometheus.MustNewConstMetric(
			allocsTotal, prometheus.GaugeValue, float64(cnt),
			k.namespace, k.clientStatus, k.desiredStatus,
		)
	}
}
