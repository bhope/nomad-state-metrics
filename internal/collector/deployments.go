package collector

import (
	"log/slog"

	nomadapi "github.com/hashicorp/nomad/api"
	"github.com/prometheus/client_golang/prometheus"
)

var (
	deploymentInfo = prometheus.NewDesc(
		"nomad_deployment_info",
		"Information about a Nomad deployment.",
		[]string{"deployment_id", "job", "namespace", "status"},
		nil,
	)
	deploymentsTotal = prometheus.NewDesc(
		"nomad_deployments_total",
		"Total number of Nomad deployments by status.",
		[]string{"namespace", "status"},
		nil,
	)
)

type deploymentCollector struct {
	client *nomadapi.Client
	logger *slog.Logger
}

func newDeploymentCollector(client *nomadapi.Client, logger *slog.Logger) *deploymentCollector {
	return &deploymentCollector{client: client, logger: logger}
}

func (c *deploymentCollector) Describe(ch chan<- *prometheus.Desc) {
	ch <- deploymentInfo
	ch <- deploymentsTotal
}

func (c *deploymentCollector) Collect(ch chan<- prometheus.Metric) {
	deploys, _, err := c.client.Deployments().List(&nomadapi.QueryOptions{Namespace: "*"})
	if err != nil {
		c.logger.Error("failed to list deployments", "error", err)
		return
	}

	type key struct{ namespace, status string }
	counts := map[key]int{}

	for _, d := range deploys {
		ch <- prometheus.MustNewConstMetric(
			deploymentInfo, prometheus.GaugeValue, 1,
			d.ID, d.JobID, d.Namespace, d.Status,
		)
		counts[key{d.Namespace, d.Status}]++
	}

	for k, cnt := range counts {
		ch <- prometheus.MustNewConstMetric(
			deploymentsTotal, prometheus.GaugeValue, float64(cnt),
			k.namespace, k.status,
		)
	}
}
