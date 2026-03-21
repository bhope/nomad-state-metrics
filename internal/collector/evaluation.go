package collector

import (
	"log/slog"

	nomadapi "github.com/hashicorp/nomad/api"
	"github.com/prometheus/client_golang/prometheus"
)

// evaluationLister is the subset of store.NomadStore used by evalMetricCollector.
type evaluationLister interface {
	ListEvaluations() []*nomadapi.Evaluation
}

var (
	evalStatusDesc = prometheus.NewDesc(
		"nomad_evaluation_status",
		"Evaluation status as a binary gauge; 1 for the active status, 0 for others.",
		[]string{"eval_id", "job_id", "namespace", "status"},
		nil,
	)
	evalFailedTGAllocsDesc = prometheus.NewDesc(
		"nomad_evaluation_failed_tg_allocs",
		"Number of failed allocation attempts for a task group in an evaluation (CoalescedFailures+1).",
		[]string{"eval_id", "job_id", "namespace", "task_group"},
		nil,
	)
)

// evalStatuses is the fixed set of statuses emitted by nomad_evaluation_status.
var evalStatuses = []string{"blocked", "pending", "complete", "failed", "canceled"}

type evalMetricCollector struct {
	store  evaluationLister
	logger *slog.Logger
}

func newEvalMetricCollector(s evaluationLister, logger *slog.Logger) *evalMetricCollector {
	return &evalMetricCollector{store: s, logger: logger}
}

func (c *evalMetricCollector) Describe(ch chan<- *prometheus.Desc) {
	ch <- evalStatusDesc
	ch <- evalFailedTGAllocsDesc
}

func (c *evalMetricCollector) Collect(ch chan<- prometheus.Metric) {
	for _, e := range c.store.ListEvaluations() {
		id, job, ns := e.ID, e.JobID, e.Namespace

		for _, s := range evalStatuses {
			ch <- prometheus.MustNewConstMetric(
				evalStatusDesc, prometheus.GaugeValue, boolFloat(e.Status == s),
				id, job, ns, s,
			)
		}

		for tg, m := range e.FailedTGAllocs {
			ch <- prometheus.MustNewConstMetric(
				evalFailedTGAllocsDesc, prometheus.GaugeValue, float64(m.CoalescedFailures+1),
				id, job, ns, tg,
			)
		}
	}
}
