package collector

import (
	"testing"

	nomadapi "github.com/hashicorp/nomad/api"
	"github.com/prometheus/client_golang/prometheus"
	dto "github.com/prometheus/client_model/go"
)

// mockAllocLister implements allocLister for tests.
type mockAllocLister struct {
	allocs []*nomadapi.AllocationListStub
}

func (m *mockAllocLister) ListAllocations() []*nomadapi.AllocationListStub { return m.allocs }

func gatherAllocs(t *testing.T, c prometheus.Collector) []*dto.MetricFamily {
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

// ── helpers to build stubs ────────────────────────────────────────────────────

func makeAlloc(jobID, ns, tg, allocID, nodeID, clientStatus, desiredStatus string,
	deployStatus *nomadapi.AllocDeploymentStatus,
	taskStates map[string]*nomadapi.TaskState,
	createTime int64,
) *nomadapi.AllocationListStub {
	return &nomadapi.AllocationListStub{
		ID:               allocID,
		JobID:            jobID,
		Namespace:        ns,
		TaskGroup:        tg,
		NodeID:           nodeID,
		ClientStatus:     clientStatus,
		DesiredStatus:    desiredStatus,
		DeploymentStatus: deployStatus,
		TaskStates:       taskStates,
		CreateTime:       createTime,
	}
}

func healthyDeployStatus() *nomadapi.AllocDeploymentStatus {
	h := true
	return &nomadapi.AllocDeploymentStatus{Healthy: &h}
}

func unhealthyDeployStatus() *nomadapi.AllocDeploymentStatus {
	h := false
	return &nomadapi.AllocDeploymentStatus{Healthy: &h}
}

// ── test cases ────────────────────────────────────────────────────────────────

func TestAllocCollector_ClientStatus(t *testing.T) {
	tests := []struct {
		clientStatus string
	}{
		{"pending"},
		{"running"},
		{"complete"},
		{"failed"},
		{"lost"},
	}

	for _, tt := range tests {
		t.Run(tt.clientStatus, func(t *testing.T) {
			a := makeAlloc("myjob", "default", "web", "alloc-1", "node-1",
				tt.clientStatus, "run", nil, nil, 1_000_000_000)
			mfs := gatherAllocs(t, newAllocCollector(&mockAllocLister{allocs: []*nomadapi.AllocationListStub{a}}, nil))

			base := map[string]string{
				"job_id": "myjob", "namespace": "default",
				"task_group": "web", "alloc_id": "alloc-1", "node_id": "node-1",
			}
			for _, s := range allocClientStatuses {
				labels := map[string]string{
					"job_id": base["job_id"], "namespace": base["namespace"],
					"task_group": base["task_group"], "alloc_id": base["alloc_id"],
					"node_id": base["node_id"], "status": s,
				}
				requireMetric(t, mfs, boolFloat(s == tt.clientStatus), "nomad_allocation_status", labels)
			}
		})
	}
}

func TestAllocCollector_DesiredStatus(t *testing.T) {
	tests := []struct {
		desiredStatus string
	}{
		{"run"},
		{"stop"},
		{"evict"},
	}

	for _, tt := range tests {
		t.Run(tt.desiredStatus, func(t *testing.T) {
			a := makeAlloc("myjob", "default", "web", "alloc-1", "node-1",
				"running", tt.desiredStatus, nil, nil, 1_000_000_000)
			mfs := gatherAllocs(t, newAllocCollector(&mockAllocLister{allocs: []*nomadapi.AllocationListStub{a}}, nil))

			for _, ds := range allocDesiredStatuses {
				labels := map[string]string{
					"job_id": "myjob", "namespace": "default",
					"task_group": "web", "alloc_id": "alloc-1",
					"desired_status": ds,
				}
				requireMetric(t, mfs, boolFloat(ds == tt.desiredStatus), "nomad_allocation_desired_status", labels)
			}
		})
	}
}

func TestAllocCollector_Health(t *testing.T) {
	tests := []struct {
		name         string
		deployStatus *nomadapi.AllocDeploymentStatus
		wantHealth   float64
	}{
		{"nil_deploy_status", nil, -1},
		{"nil_healthy_pointer", &nomadapi.AllocDeploymentStatus{Healthy: nil}, -1},
		{"healthy", healthyDeployStatus(), 1},
		{"unhealthy", unhealthyDeployStatus(), 0},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			a := makeAlloc("myjob", "default", "web", "alloc-1", "node-1",
				"running", "run", tt.deployStatus, nil, 1_000_000_000)
			mfs := gatherAllocs(t, newAllocCollector(&mockAllocLister{allocs: []*nomadapi.AllocationListStub{a}}, nil))

			requireMetric(t, mfs, tt.wantHealth, "nomad_allocation_health", map[string]string{
				"job_id": "myjob", "namespace": "default",
				"task_group": "web", "alloc_id": "alloc-1",
			})
		})
	}
}

func TestAllocCollector_CreatedTimestamp(t *testing.T) {
	tests := []struct {
		name           string
		createTimeNs   int64
		wantTimestampS float64
	}{
		{"epoch", 0, 0},
		{"one_second", 1_000_000_000, 1},
		{"arbitrary", 1_711_929_600_000_000_000, 1_711_929_600},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			a := makeAlloc("myjob", "default", "web", "alloc-1", "node-1",
				"running", "run", nil, nil, tt.createTimeNs)
			mfs := gatherAllocs(t, newAllocCollector(&mockAllocLister{allocs: []*nomadapi.AllocationListStub{a}}, nil))

			requireMetric(t, mfs, tt.wantTimestampS, "nomad_allocation_created_timestamp", map[string]string{
				"job_id": "myjob", "namespace": "default",
				"task_group": "web", "alloc_id": "alloc-1",
			})
		})
	}
}

func TestAllocCollector_TaskMetrics(t *testing.T) {
	tests := []struct {
		name         string
		taskName     string
		taskState    string
		restarts     uint64
		wantRestarts float64
	}{
		{"running_no_restarts", "server", "running", 0, 0},
		{"dead_with_restarts", "worker", "dead", 3, 3},
		{"pending_one_restart", "init", "pending", 1, 1},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			taskStates := map[string]*nomadapi.TaskState{
				tt.taskName: {State: tt.taskState, Restarts: tt.restarts},
			}
			a := makeAlloc("myjob", "default", "web", "alloc-1", "node-1",
				"running", "run", nil, taskStates, 1_000_000_000)
			mfs := gatherAllocs(t, newAllocCollector(&mockAllocLister{allocs: []*nomadapi.AllocationListStub{a}}, nil))

			// restart count
			requireMetric(t, mfs, tt.wantRestarts, "nomad_allocation_restart_count", map[string]string{
				"job_id": "myjob", "namespace": "default",
				"task_group": "web", "alloc_id": "alloc-1", "task_name": tt.taskName,
			})

			// task state: active state = 1, others = 0
			for _, state := range allocTaskStates {
				requireMetric(t, mfs, boolFloat(state == tt.taskState), "nomad_allocation_task_state", map[string]string{
					"job_id": "myjob", "namespace": "default",
					"task_group": "web", "alloc_id": "alloc-1",
					"task_name": tt.taskName, "state": state,
				})
			}
		})
	}
}

func TestAllocCollector_MultipleTasksPerAlloc(t *testing.T) {
	taskStates := map[string]*nomadapi.TaskState{
		"api":     {State: "running", Restarts: 0},
		"sidecar": {State: "running", Restarts: 1},
	}
	a := makeAlloc("platform", "infra", "api", "alloc-2", "node-2",
		"running", "run", healthyDeployStatus(), taskStates, 2_000_000_000)
	mfs := gatherAllocs(t, newAllocCollector(&mockAllocLister{allocs: []*nomadapi.AllocationListStub{a}}, nil))

	requireMetric(t, mfs, 0, "nomad_allocation_restart_count", map[string]string{
		"job_id": "platform", "namespace": "infra",
		"task_group": "api", "alloc_id": "alloc-2", "task_name": "api",
	})
	requireMetric(t, mfs, 1, "nomad_allocation_restart_count", map[string]string{
		"job_id": "platform", "namespace": "infra",
		"task_group": "api", "alloc_id": "alloc-2", "task_name": "sidecar",
	})
	requireMetric(t, mfs, 1, "nomad_allocation_health", map[string]string{
		"job_id": "platform", "namespace": "infra",
		"task_group": "api", "alloc_id": "alloc-2",
	})
}

func TestAllocCollector_MultipleAllocs(t *testing.T) {
	allocs := []*nomadapi.AllocationListStub{
		makeAlloc("jobA", "ns1", "web", "alloc-a", "node-1", "running", "run", healthyDeployStatus(), nil, 1_000_000_000),
		makeAlloc("jobB", "ns2", "worker", "alloc-b", "node-2", "failed", "stop", unhealthyDeployStatus(), nil, 2_000_000_000),
	}
	mfs := gatherAllocs(t, newAllocCollector(&mockAllocLister{allocs: allocs}, nil))

	requireMetric(t, mfs, 1, "nomad_allocation_status", map[string]string{
		"job_id": "jobA", "namespace": "ns1", "task_group": "web",
		"alloc_id": "alloc-a", "node_id": "node-1", "status": "running",
	})
	requireMetric(t, mfs, 1, "nomad_allocation_status", map[string]string{
		"job_id": "jobB", "namespace": "ns2", "task_group": "worker",
		"alloc_id": "alloc-b", "node_id": "node-2", "status": "failed",
	})
	requireMetric(t, mfs, 1, "nomad_allocation_health", map[string]string{
		"job_id": "jobA", "namespace": "ns1", "task_group": "web", "alloc_id": "alloc-a",
	})
	requireMetric(t, mfs, 0, "nomad_allocation_health", map[string]string{
		"job_id": "jobB", "namespace": "ns2", "task_group": "worker", "alloc_id": "alloc-b",
	})
}

func TestAllocCollector_EmptyCluster(t *testing.T) {
	mfs := gatherAllocs(t, newAllocCollector(&mockAllocLister{allocs: nil}, nil))

	for _, name := range []string{
		"nomad_allocation_status", "nomad_allocation_desired_status",
		"nomad_allocation_health", "nomad_allocation_restart_count",
		"nomad_allocation_created_timestamp", "nomad_allocation_task_state",
	} {
		requireAbsent(t, mfs, name)
	}
}
