package collector

import (
	"log/slog"

	nomadapi "github.com/hashicorp/nomad/api"
	"github.com/prometheus/client_golang/prometheus"
)

// jobLister is the subset of store.NomadStore used by jobCollector.
type jobLister interface {
	ListJobs() []*nomadapi.JobListStub
}

var (
	jobInfoDesc = prometheus.NewDesc(
		"nomad_job_info",
		"Static information about a Nomad job.",
		[]string{"job_id", "namespace", "job_type", "status", "priority"},
		nil,
	)
	jobStatusDesc = prometheus.NewDesc(
		"nomad_job_status",
		"Current job status as a binary gauge; 1 for the active status, 0 for others.",
		[]string{"job_id", "namespace", "status"},
		nil,
	)
	jobTaskGroupsDesiredDesc = prometheus.NewDesc(
		"nomad_job_task_groups_desired",
		"Number of desired (queued+starting+running) allocations for a task group.",
		[]string{"job_id", "namespace", "task_group"},
		nil,
	)
	jobTaskGroupsRunningDesc = prometheus.NewDesc(
		"nomad_job_task_groups_running",
		"Number of running allocations for a task group.",
		[]string{"job_id", "namespace", "task_group"},
		nil,
	)
	jobVersionDesc = prometheus.NewDesc(
		"nomad_job_version",
		"JobModifyIndex of the job, used as a version proxy.",
		[]string{"job_id", "namespace"},
		nil,
	)
	jobIsPeriodicDesc = prometheus.NewDesc(
		"nomad_job_is_periodic",
		"1 if the job is periodic, 0 otherwise.",
		[]string{"job_id", "namespace"},
		nil,
	)
	jobIsParameterizedDesc = prometheus.NewDesc(
		"nomad_job_is_parameterized",
		"1 if the job is parameterized, 0 otherwise.",
		[]string{"job_id", "namespace"},
		nil,
	)
)

// jobStatuses is the fixed set of statuses emitted by nomad_job_status.
var jobStatuses = []string{"running", "pending", "dead"}

type jobCollector struct {
	store  jobLister
	logger *slog.Logger
}

func newJobCollector(s jobLister, logger *slog.Logger) *jobCollector {
	return &jobCollector{store: s, logger: logger}
}

func (c *jobCollector) Describe(ch chan<- *prometheus.Desc) {
	ch <- jobInfoDesc
	ch <- jobStatusDesc
	ch <- jobTaskGroupsDesiredDesc
	ch <- jobTaskGroupsRunningDesc
	ch <- jobVersionDesc
	ch <- jobIsPeriodicDesc
	ch <- jobIsParameterizedDesc
}

func (c *jobCollector) Collect(ch chan<- prometheus.Metric) {
	for _, j := range c.store.ListJobs() {
		id, ns := j.ID, j.Namespace

		ch <- prometheus.MustNewConstMetric(
			jobInfoDesc, prometheus.GaugeValue, 1,
			id, ns, j.Type, j.Status, itoa(j.Priority),
		)

		for _, s := range jobStatuses {
			ch <- prometheus.MustNewConstMetric(
				jobStatusDesc, prometheus.GaugeValue, boolFloat(j.Status == s),
				id, ns, s,
			)
		}

		if j.JobSummary != nil {
			for tg, sum := range j.JobSummary.Summary {
				desired := sum.Queued + sum.Starting + sum.Running
				ch <- prometheus.MustNewConstMetric(
					jobTaskGroupsDesiredDesc, prometheus.GaugeValue, float64(desired),
					id, ns, tg,
				)
				ch <- prometheus.MustNewConstMetric(
					jobTaskGroupsRunningDesc, prometheus.GaugeValue, float64(sum.Running),
					id, ns, tg,
				)
			}
		}

		ch <- prometheus.MustNewConstMetric(
			jobVersionDesc, prometheus.GaugeValue, float64(j.JobModifyIndex),
			id, ns,
		)
		ch <- prometheus.MustNewConstMetric(
			jobIsPeriodicDesc, prometheus.GaugeValue, boolFloat(j.Periodic),
			id, ns,
		)
		ch <- prometheus.MustNewConstMetric(
			jobIsParameterizedDesc, prometheus.GaugeValue, boolFloat(j.ParameterizedJob),
			id, ns,
		)
	}
}

func boolFloat(b bool) float64 {
	if b {
		return 1
	}
	return 0
}
