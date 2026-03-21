package collector

import (
	"log/slog"

	nomadapi "github.com/hashicorp/nomad/api"
	"github.com/prometheus/client_golang/prometheus"
)

// deploymentLister is the subset of store.NomadStore used by deploymentMetricCollector.
type deploymentLister interface {
	ListDeployments() []*nomadapi.Deployment
}

var (
	deployStatusDesc = prometheus.NewDesc(
		"nomad_deployment_status",
		"Deployment status as a binary gauge; 1 for the active status, 0 for others.",
		[]string{"deployment_id", "job_id", "namespace", "status"},
		nil,
	)
	deployTGDesiredDesc = prometheus.NewDesc(
		"nomad_deployment_task_group_desired",
		"Desired number of allocations for a task group in a deployment.",
		[]string{"deployment_id", "job_id", "namespace", "task_group"},
		nil,
	)
	deployTGPlacedDesc = prometheus.NewDesc(
		"nomad_deployment_task_group_placed",
		"Number of placed allocations for a task group in a deployment.",
		[]string{"deployment_id", "job_id", "namespace", "task_group"},
		nil,
	)
	deployTGHealthyDesc = prometheus.NewDesc(
		"nomad_deployment_task_group_healthy",
		"Number of healthy allocations for a task group in a deployment.",
		[]string{"deployment_id", "job_id", "namespace", "task_group"},
		nil,
	)
	deployTGUnhealthyDesc = prometheus.NewDesc(
		"nomad_deployment_task_group_unhealthy",
		"Number of unhealthy allocations for a task group in a deployment.",
		[]string{"deployment_id", "job_id", "namespace", "task_group"},
		nil,
	)
	deployAutoRevertDesc = prometheus.NewDesc(
		"nomad_deployment_auto_revert",
		"1 if any task group in the deployment has auto-revert enabled, 0 otherwise.",
		[]string{"deployment_id", "job_id", "namespace"},
		nil,
	)
)

// deploymentStatuses is the fixed set of statuses emitted by nomad_deployment_status.
var deploymentStatuses = []string{"running", "successful", "cancelled", "failed"}

type deploymentMetricCollector struct {
	store  deploymentLister
	logger *slog.Logger
}

func newDeploymentMetricCollector(s deploymentLister, logger *slog.Logger) *deploymentMetricCollector {
	return &deploymentMetricCollector{store: s, logger: logger}
}

func (c *deploymentMetricCollector) Describe(ch chan<- *prometheus.Desc) {
	ch <- deployStatusDesc
	ch <- deployTGDesiredDesc
	ch <- deployTGPlacedDesc
	ch <- deployTGHealthyDesc
	ch <- deployTGUnhealthyDesc
	ch <- deployAutoRevertDesc
}

func (c *deploymentMetricCollector) Collect(ch chan<- prometheus.Metric) {
	for _, d := range c.store.ListDeployments() {
		id, job, ns := d.ID, d.JobID, d.Namespace

		for _, s := range deploymentStatuses {
			ch <- prometheus.MustNewConstMetric(
				deployStatusDesc, prometheus.GaugeValue, boolFloat(d.Status == s),
				id, job, ns, s,
			)
		}

		autoRevert := 0.0
		for tg, state := range d.TaskGroups {
			ch <- prometheus.MustNewConstMetric(
				deployTGDesiredDesc, prometheus.GaugeValue, float64(state.DesiredTotal),
				id, job, ns, tg,
			)
			ch <- prometheus.MustNewConstMetric(
				deployTGPlacedDesc, prometheus.GaugeValue, float64(state.PlacedAllocs),
				id, job, ns, tg,
			)
			ch <- prometheus.MustNewConstMetric(
				deployTGHealthyDesc, prometheus.GaugeValue, float64(state.HealthyAllocs),
				id, job, ns, tg,
			)
			ch <- prometheus.MustNewConstMetric(
				deployTGUnhealthyDesc, prometheus.GaugeValue, float64(state.UnhealthyAllocs),
				id, job, ns, tg,
			)
			if state.AutoRevert {
				autoRevert = 1
			}
		}

		ch <- prometheus.MustNewConstMetric(
			deployAutoRevertDesc, prometheus.GaugeValue, autoRevert,
			id, job, ns,
		)
	}
}
