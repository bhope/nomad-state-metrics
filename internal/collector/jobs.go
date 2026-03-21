package collector

import (
	"log/slog"

	nomadapi "github.com/hashicorp/nomad/api"
	"github.com/prometheus/client_golang/prometheus"
)

var (
	jobInfo = prometheus.NewDesc(
		"nomad_job_info",
		"Information about a Nomad job.",
		[]string{"job", "namespace", "type", "status", "priority"},
		nil,
	)
	jobStatusCount = prometheus.NewDesc(
		"nomad_jobs_total",
		"Total number of Nomad jobs by status.",
		[]string{"namespace", "status"},
		nil,
	)
)

type jobCollector struct {
	client *nomadapi.Client
	logger *slog.Logger
}

func newJobCollector(client *nomadapi.Client, logger *slog.Logger) *jobCollector {
	return &jobCollector{client: client, logger: logger}
}

func (c *jobCollector) Describe(ch chan<- *prometheus.Desc) {
	ch <- jobInfo
	ch <- jobStatusCount
}

func (c *jobCollector) Collect(ch chan<- prometheus.Metric) {
	jobs, _, err := c.client.Jobs().List(&nomadapi.QueryOptions{Namespace: "*"})
	if err != nil {
		c.logger.Error("failed to list jobs", "error", err)
		return
	}

	counts := map[string]map[string]int{} // namespace -> status -> count

	for _, j := range jobs {
		ch <- prometheus.MustNewConstMetric(
			jobInfo, prometheus.GaugeValue, 1,
			j.ID, j.Namespace, j.Type, j.Status, itoa(j.Priority),
		)
		if _, ok := counts[j.Namespace]; !ok {
			counts[j.Namespace] = map[string]int{}
		}
		counts[j.Namespace][j.Status]++
	}

	for ns, byStatus := range counts {
		for status, n := range byStatus {
			ch <- prometheus.MustNewConstMetric(
				jobStatusCount, prometheus.GaugeValue, float64(n),
				ns, status,
			)
		}
	}
}
