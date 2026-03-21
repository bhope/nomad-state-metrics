package collector

import (
	"log/slog"

	nomadapi "github.com/hashicorp/nomad/api"
	"github.com/prometheus/client_golang/prometheus"
)

// allocLister is the subset of store.NomadStore used by allocCollector.
type allocLister interface {
	ListAllocations() []*nomadapi.AllocationListStub
}

var (
	allocStatusDesc = prometheus.NewDesc(
		"nomad_allocation_status",
		"Allocation client status as a binary gauge; 1 for the active status, 0 for others.",
		[]string{"job_id", "namespace", "task_group", "alloc_id", "node_id", "status"},
		nil,
	)
	allocDesiredStatusDesc = prometheus.NewDesc(
		"nomad_allocation_desired_status",
		"Allocation desired status as a binary gauge; 1 for the active desired status, 0 for others.",
		[]string{"job_id", "namespace", "task_group", "alloc_id", "desired_status"},
		nil,
	)
	allocHealthDesc = prometheus.NewDesc(
		"nomad_allocation_health",
		"Allocation deployment health: 1=healthy, 0=unhealthy, -1=unknown.",
		[]string{"job_id", "namespace", "task_group", "alloc_id"},
		nil,
	)
	allocRestartCountDesc = prometheus.NewDesc(
		"nomad_allocation_restart_count",
		"Number of restarts for a task within an allocation.",
		[]string{"job_id", "namespace", "task_group", "alloc_id", "task_name"},
		nil,
	)
	allocCreatedTimestampDesc = prometheus.NewDesc(
		"nomad_allocation_created_timestamp",
		"Unix timestamp (seconds) of when the allocation was created.",
		[]string{"job_id", "namespace", "task_group", "alloc_id"},
		nil,
	)
	allocTaskStateDesc = prometheus.NewDesc(
		"nomad_allocation_task_state",
		"Task state within an allocation as a binary gauge; 1 for the active state, 0 for others.",
		[]string{"job_id", "namespace", "task_group", "alloc_id", "task_name", "state"},
		nil,
	)
)

// allocClientStatuses is the fixed set of client statuses emitted by nomad_allocation_status.
var allocClientStatuses = []string{"pending", "running", "complete", "failed", "lost"}

// allocDesiredStatuses is the fixed set of desired statuses emitted by nomad_allocation_desired_status.
var allocDesiredStatuses = []string{"run", "stop", "evict"}

// allocTaskStates is the fixed set of task states emitted by nomad_allocation_task_state.
var allocTaskStates = []string{"pending", "running", "dead"}

type allocCollector struct {
	store  allocLister
	logger *slog.Logger
}

func newAllocCollector(s allocLister, logger *slog.Logger) *allocCollector {
	return &allocCollector{store: s, logger: logger}
}

func (c *allocCollector) Describe(ch chan<- *prometheus.Desc) {
	ch <- allocStatusDesc
	ch <- allocDesiredStatusDesc
	ch <- allocHealthDesc
	ch <- allocRestartCountDesc
	ch <- allocCreatedTimestampDesc
	ch <- allocTaskStateDesc
}

func (c *allocCollector) Collect(ch chan<- prometheus.Metric) {
	for _, a := range c.store.ListAllocations() {
		job, ns, tg, id, node := a.JobID, a.Namespace, a.TaskGroup, a.ID, a.NodeID

		for _, s := range allocClientStatuses {
			ch <- prometheus.MustNewConstMetric(
				allocStatusDesc, prometheus.GaugeValue, boolFloat(a.ClientStatus == s),
				job, ns, tg, id, node, s,
			)
		}

		for _, ds := range allocDesiredStatuses {
			ch <- prometheus.MustNewConstMetric(
				allocDesiredStatusDesc, prometheus.GaugeValue, boolFloat(a.DesiredStatus == ds),
				job, ns, tg, id, ds,
			)
		}

		ch <- prometheus.MustNewConstMetric(
			allocHealthDesc, prometheus.GaugeValue, allocDeploymentHealth(a.DeploymentStatus),
			job, ns, tg, id,
		)

		// CreateTime is nanoseconds; convert to Unix seconds.
		ch <- prometheus.MustNewConstMetric(
			allocCreatedTimestampDesc, prometheus.GaugeValue, float64(a.CreateTime)/1e9,
			job, ns, tg, id,
		)

		for taskName, ts := range a.TaskStates {
			ch <- prometheus.MustNewConstMetric(
				allocRestartCountDesc, prometheus.GaugeValue, float64(ts.Restarts),
				job, ns, tg, id, taskName,
			)

			for _, state := range allocTaskStates {
				ch <- prometheus.MustNewConstMetric(
					allocTaskStateDesc, prometheus.GaugeValue, boolFloat(ts.State == state),
					job, ns, tg, id, taskName, state,
				)
			}
		}
	}
}

func allocDeploymentHealth(ds *nomadapi.AllocDeploymentStatus) float64 {
	if ds == nil || ds.Healthy == nil {
		return -1
	}
	if *ds.Healthy {
		return 1
	}
	return 0
}
