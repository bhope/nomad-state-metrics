package collector

import (
	"log/slog"

	nomadapi "github.com/hashicorp/nomad/api"
	"github.com/prometheus/client_golang/prometheus"
)

var (
	nodeInfo = prometheus.NewDesc(
		"nomad_node_info",
		"Information about a Nomad node.",
		[]string{"node", "node_id", "datacenter", "class", "drain", "eligibility", "status"},
		nil,
	)
	nodesTotal = prometheus.NewDesc(
		"nomad_nodes_total",
		"Total number of Nomad nodes by status and eligibility.",
		[]string{"status", "eligibility"},
		nil,
	)
)

type nodeCollector struct {
	client *nomadapi.Client
	logger *slog.Logger
}

func newNodeCollector(client *nomadapi.Client, logger *slog.Logger) *nodeCollector {
	return &nodeCollector{client: client, logger: logger}
}

func (c *nodeCollector) Describe(ch chan<- *prometheus.Desc) {
	ch <- nodeInfo
	ch <- nodesTotal
}

func (c *nodeCollector) Collect(ch chan<- prometheus.Metric) {
	nodes, _, err := c.client.Nodes().List(nil)
	if err != nil {
		c.logger.Error("failed to list nodes", "error", err)
		return
	}

	type key struct{ status, eligibility string }
	counts := map[key]int{}

	for _, n := range nodes {
		ch <- prometheus.MustNewConstMetric(
			nodeInfo, prometheus.GaugeValue, 1,
			n.Name, n.ID, n.Datacenter, n.NodeClass,
			boolStr(n.Drain), n.SchedulingEligibility, n.Status,
		)
		counts[key{n.Status, n.SchedulingEligibility}]++
	}

	for k, cnt := range counts {
		ch <- prometheus.MustNewConstMetric(
			nodesTotal, prometheus.GaugeValue, float64(cnt),
			k.status, k.eligibility,
		)
	}
}
