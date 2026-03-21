package collector

import (
	"log/slog"

	"github.com/prometheus/client_golang/prometheus"

	"github.com/bhope/nomad-state-metrics/internal/store"
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
	store  *store.NomadStore
	logger *slog.Logger
}

func newEvaluationCollector(s *store.NomadStore, logger *slog.Logger) *evaluationCollector {
	return &evaluationCollector{store: s, logger: logger}
}

func (c *evaluationCollector) Describe(ch chan<- *prometheus.Desc) {
	ch <- evalInfo
	ch <- evalsTotal
}

func (c *evaluationCollector) Collect(ch chan<- prometheus.Metric) {
	evals := c.store.ListEvaluations()

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
