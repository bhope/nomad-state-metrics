package collector

import (
	"log/slog"

	nomadapi "github.com/hashicorp/nomad/api"
	"github.com/prometheus/client_golang/prometheus"
)

var (
	evalInfo = prometheus.NewDesc(
		"nomad_evaluation_info",
		"Information about a Nomad evaluation.",
		[]string{"eval_id", "job", "namespace", "status", "triggered_by", "type"},
		nil,
	)
	evalsTotal = prometheus.NewDesc(
		"nomad_evaluations_total",
		"Total number of Nomad evaluations by status.",
		[]string{"namespace", "status"},
		nil,
	)
)

type evaluationCollector struct {
	client *nomadapi.Client
	logger *slog.Logger
}

func newEvaluationCollector(client *nomadapi.Client, logger *slog.Logger) *evaluationCollector {
	return &evaluationCollector{client: client, logger: logger}
}

func (c *evaluationCollector) Describe(ch chan<- *prometheus.Desc) {
	ch <- evalInfo
	ch <- evalsTotal
}

func (c *evaluationCollector) Collect(ch chan<- prometheus.Metric) {
	evals, _, err := c.client.Evaluations().List(&nomadapi.QueryOptions{Namespace: "*"})
	if err != nil {
		c.logger.Error("failed to list evaluations", "error", err)
		return
	}

	type key struct{ namespace, status string }
	counts := map[key]int{}

	for _, e := range evals {
		ch <- prometheus.MustNewConstMetric(
			evalInfo, prometheus.GaugeValue, 1,
			e.ID, e.JobID, e.Namespace, e.Status, e.TriggeredBy, e.Type,
		)
		counts[key{e.Namespace, e.Status}]++
	}

	for k, cnt := range counts {
		ch <- prometheus.MustNewConstMetric(
			evalsTotal, prometheus.GaugeValue, float64(cnt),
			k.namespace, k.status,
		)
	}
}
