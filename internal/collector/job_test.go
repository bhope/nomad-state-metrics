package collector

import (
	"testing"

	nomadapi "github.com/hashicorp/nomad/api"
	dto "github.com/prometheus/client_model/go"
	"github.com/prometheus/client_golang/prometheus"
)

// mockJobLister implements jobLister for tests.
type mockJobLister struct {
	jobs []*nomadapi.JobListStub
}

func (m *mockJobLister) ListJobs() []*nomadapi.JobListStub { return m.jobs }

// gatherJobs registers c in a fresh registry, gathers, and returns a flat map
// keyed by "metric_name\x00label1=v1\x00label2=v2\x00..." (sorted by label name
// as Prometheus guarantees label order matches the descriptor's variableLabels).
func gatherJobs(t *testing.T, c prometheus.Collector) []*dto.MetricFamily {
	t.Helper()
	reg := prometheus.NewRegistry()
	if err := reg.Register(c); err != nil {
		t.Fatalf("register: %v", err)
	}
	mfs, err := reg.Gather()
	if err != nil {
		t.Fatalf("gather: %v", err)
	}
	return mfs
}

// metricValue finds the gauge value for the named metric whose labels exactly
// match the provided map. Prometheus sorts label pairs alphabetically in
// gathered output, so we match by name rather than position.
func metricValue(mfs []*dto.MetricFamily, name string, labels map[string]string) (float64, bool) {
	for _, mf := range mfs {
		if mf.GetName() != name {
			continue
		}
		for _, m := range mf.GetMetric() {
			pairs := m.GetLabel()
			if len(pairs) != len(labels) {
				continue
			}
			match := true
			for _, p := range pairs {
				if want, ok := labels[p.GetName()]; !ok || p.GetValue() != want {
					match = false
					break
				}
			}
			if match {
				return m.GetGauge().GetValue(), true
			}
		}
	}
	return 0, false
}

func requireMetric(t *testing.T, mfs []*dto.MetricFamily, want float64, name string, labels map[string]string) {
	t.Helper()
	got, ok := metricValue(mfs, name, labels)
	if !ok {
		t.Errorf("metric %s%v not found", name, labels)
		return
	}
	if got != want {
		t.Errorf("metric %s%v = %g, want %g", name, labels, got, want)
	}
}

func requireAbsent(t *testing.T, mfs []*dto.MetricFamily, name string) {
	t.Helper()
	for _, mf := range mfs {
		if mf.GetName() == name && len(mf.GetMetric()) > 0 {
			t.Errorf("metric %s unexpectedly present", name)
			return
		}
	}
}

// ── helpers to build stubs ────────────────────────────────────────────────────

func serviceJob(id, ns, status string, jobModifyIndex uint64, tgs map[string]nomadapi.TaskGroupSummary) *nomadapi.JobListStub {
	return &nomadapi.JobListStub{
		ID:             id,
		Namespace:      ns,
		Type:           "service",
		Status:         status,
		Priority:       50,
		JobModifyIndex: jobModifyIndex,
		JobSummary: &nomadapi.JobSummary{
			JobID:     id,
			Namespace: ns,
			Summary:   tgs,
		},
	}
}

func batchJob(id, ns, status string) *nomadapi.JobListStub {
	j := serviceJob(id, ns, status, 42, map[string]nomadapi.TaskGroupSummary{
		"batch": {Running: 0, Queued: 0, Starting: 0},
	})
	j.Type = "batch"
	return j
}

func periodicJob(id, ns string) *nomadapi.JobListStub {
	j := serviceJob(id, ns, "running", 1, map[string]nomadapi.TaskGroupSummary{
		"cron": {Running: 1, Queued: 0, Starting: 0},
	})
	j.Periodic = true
	return j
}

func parameterizedJob(id, ns string) *nomadapi.JobListStub {
	j := serviceJob(id, ns, "running", 1, map[string]nomadapi.TaskGroupSummary{
		"dispatch": {Running: 2, Queued: 0, Starting: 0},
	})
	j.ParameterizedJob = true
	return j
}

// ── test cases ────────────────────────────────────────────────────────────────

func TestJobCollector_SingleRunningServiceJob(t *testing.T) {
	jobs := []*nomadapi.JobListStub{
		serviceJob("web", "default", "running", 3, map[string]nomadapi.TaskGroupSummary{
			"web": {Running: 2, Queued: 1, Starting: 0},
		}),
	}
	mfs := gatherJobs(t, newJobCollector(&mockJobLister{jobs: jobs}, nil))

	jobNS := map[string]string{"job_id": "web", "namespace": "default"}

	// info
	requireMetric(t, mfs, 1, "nomad_job_info", map[string]string{
		"job_id": "web", "namespace": "default", "job_type": "service", "status": "running", "priority": "50",
	})

	// status: running=1, others=0
	requireMetric(t, mfs, 1, "nomad_job_status", map[string]string{"job_id": "web", "namespace": "default", "status": "running"})
	requireMetric(t, mfs, 0, "nomad_job_status", map[string]string{"job_id": "web", "namespace": "default", "status": "pending"})
	requireMetric(t, mfs, 0, "nomad_job_status", map[string]string{"job_id": "web", "namespace": "default", "status": "dead"})

	// task group: desired = running(2)+queued(1)+starting(0) = 3
	requireMetric(t, mfs, 3, "nomad_job_task_groups_desired", map[string]string{"job_id": "web", "namespace": "default", "task_group": "web"})
	requireMetric(t, mfs, 2, "nomad_job_task_groups_running", map[string]string{"job_id": "web", "namespace": "default", "task_group": "web"})

	// version, periodic, parameterized
	requireMetric(t, mfs, 3, "nomad_job_version", jobNS)
	requireMetric(t, mfs, 0, "nomad_job_is_periodic", jobNS)
	requireMetric(t, mfs, 0, "nomad_job_is_parameterized", jobNS)
}

func TestJobCollector_DeadBatchJob(t *testing.T) {
	jobs := []*nomadapi.JobListStub{batchJob("indexer", "ops", "dead")}
	mfs := gatherJobs(t, newJobCollector(&mockJobLister{jobs: jobs}, nil))

	requireMetric(t, mfs, 1, "nomad_job_info", map[string]string{
		"job_id": "indexer", "namespace": "ops", "job_type": "batch", "status": "dead", "priority": "50",
	})

	batchNS := map[string]string{"job_id": "indexer", "namespace": "ops"}
	requireMetric(t, mfs, 0, "nomad_job_status", map[string]string{"job_id": "indexer", "namespace": "ops", "status": "running"})
	requireMetric(t, mfs, 0, "nomad_job_status", map[string]string{"job_id": "indexer", "namespace": "ops", "status": "pending"})
	requireMetric(t, mfs, 1, "nomad_job_status", map[string]string{"job_id": "indexer", "namespace": "ops", "status": "dead"})

	requireMetric(t, mfs, 0, "nomad_job_task_groups_desired", map[string]string{"job_id": "indexer", "namespace": "ops", "task_group": "batch"})
	requireMetric(t, mfs, 0, "nomad_job_task_groups_running", map[string]string{"job_id": "indexer", "namespace": "ops", "task_group": "batch"})
	requireMetric(t, mfs, 42, "nomad_job_version", batchNS)
}

func TestJobCollector_PeriodicJob(t *testing.T) {
	jobs := []*nomadapi.JobListStub{periodicJob("nightly-backup", "default")}
	mfs := gatherJobs(t, newJobCollector(&mockJobLister{jobs: jobs}, nil))

	periodicNS := map[string]string{"job_id": "nightly-backup", "namespace": "default"}
	requireMetric(t, mfs, 1, "nomad_job_is_periodic", periodicNS)
	requireMetric(t, mfs, 0, "nomad_job_is_parameterized", periodicNS)
}

func TestJobCollector_ParameterizedJob(t *testing.T) {
	jobs := []*nomadapi.JobListStub{parameterizedJob("render", "default")}
	mfs := gatherJobs(t, newJobCollector(&mockJobLister{jobs: jobs}, nil))

	renderNS := map[string]string{"job_id": "render", "namespace": "default"}
	requireMetric(t, mfs, 0, "nomad_job_is_periodic", renderNS)
	requireMetric(t, mfs, 1, "nomad_job_is_parameterized", renderNS)
}

func TestJobCollector_MultipleTaskGroups(t *testing.T) {
	jobs := []*nomadapi.JobListStub{
		serviceJob("platform", "infra", "running", 5, map[string]nomadapi.TaskGroupSummary{
			"api":    {Running: 3, Queued: 0, Starting: 1},
			"worker": {Running: 5, Queued: 2, Starting: 0},
			"cache":  {Running: 1, Queued: 0, Starting: 0},
		}),
	}
	mfs := gatherJobs(t, newJobCollector(&mockJobLister{jobs: jobs}, nil))

	tg := func(name string) map[string]string {
		return map[string]string{"job_id": "platform", "namespace": "infra", "task_group": name}
	}

	// api: desired=4 (3 running + 1 starting), running=3
	requireMetric(t, mfs, 4, "nomad_job_task_groups_desired", tg("api"))
	requireMetric(t, mfs, 3, "nomad_job_task_groups_running", tg("api"))

	// worker: desired=7 (5 running + 2 queued), running=5
	requireMetric(t, mfs, 7, "nomad_job_task_groups_desired", tg("worker"))
	requireMetric(t, mfs, 5, "nomad_job_task_groups_running", tg("worker"))

	// cache: desired=1, running=1
	requireMetric(t, mfs, 1, "nomad_job_task_groups_desired", tg("cache"))
	requireMetric(t, mfs, 1, "nomad_job_task_groups_running", tg("cache"))
}

func TestJobCollector_EmptyCluster(t *testing.T) {
	mfs := gatherJobs(t, newJobCollector(&mockJobLister{jobs: nil}, nil))

	for _, name := range []string{
		"nomad_job_info", "nomad_job_status",
		"nomad_job_task_groups_desired", "nomad_job_task_groups_running",
		"nomad_job_version", "nomad_job_is_periodic", "nomad_job_is_parameterized",
	} {
		requireAbsent(t, mfs, name)
	}
}
