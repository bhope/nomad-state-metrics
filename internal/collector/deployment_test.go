package collector

import (
	"testing"

	nomadapi "github.com/hashicorp/nomad/api"
	dto "github.com/prometheus/client_model/go"
	"github.com/prometheus/client_golang/prometheus"
)

// mockDeploymentLister implements deploymentLister for tests.
type mockDeploymentLister struct {
	deployments []*nomadapi.Deployment
}

func (m *mockDeploymentLister) ListDeployments() []*nomadapi.Deployment { return m.deployments }

func gatherDeployments(t *testing.T, c prometheus.Collector) []*dto.MetricFamily {
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

func makeDeployment(id, jobID, ns, status string, tgs map[string]*nomadapi.DeploymentState) *nomadapi.Deployment {
	return &nomadapi.Deployment{
		ID:         id,
		JobID:      jobID,
		Namespace:  ns,
		Status:     status,
		TaskGroups: tgs,
	}
}

func deployState(desired, placed, healthy, unhealthy int, autoRevert bool) *nomadapi.DeploymentState {
	return &nomadapi.DeploymentState{
		DesiredTotal:    desired,
		PlacedAllocs:    placed,
		HealthyAllocs:   healthy,
		UnhealthyAllocs: unhealthy,
		AutoRevert:      autoRevert,
	}
}

// ── test cases ────────────────────────────────────────────────────────────────

func TestDeploymentCollector_Status(t *testing.T) {
	tests := []struct {
		status string
	}{
		{"running"},
		{"successful"},
		{"cancelled"},
		{"failed"},
	}

	for _, tt := range tests {
		t.Run(tt.status, func(t *testing.T) {
			d := makeDeployment("dep-1", "myjob", "default", tt.status, nil)
			mfs := gatherDeployments(t, newDeploymentMetricCollector(&mockDeploymentLister{deployments: []*nomadapi.Deployment{d}}, nil))

			for _, s := range deploymentStatuses {
				requireMetric(t, mfs, boolFloat(s == tt.status), "nomad_deployment_status", map[string]string{
					"deployment_id": "dep-1", "job_id": "myjob",
					"namespace": "default", "status": s,
				})
			}
		})
	}
}

func TestDeploymentCollector_TaskGroupCounts(t *testing.T) {
	tests := []struct {
		name            string
		desired         int
		placed          int
		healthy         int
		unhealthy       int
	}{
		{"all_healthy", 3, 3, 3, 0},
		{"rolling_update", 5, 4, 3, 1},
		{"fresh_deployment", 2, 0, 0, 0},
		{"all_unhealthy", 3, 3, 0, 3},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tgs := map[string]*nomadapi.DeploymentState{
				"web": deployState(tt.desired, tt.placed, tt.healthy, tt.unhealthy, false),
			}
			d := makeDeployment("dep-1", "myjob", "default", "running", tgs)
			mfs := gatherDeployments(t, newDeploymentMetricCollector(&mockDeploymentLister{deployments: []*nomadapi.Deployment{d}}, nil))

			tgLabels := map[string]string{
				"deployment_id": "dep-1", "job_id": "myjob",
				"namespace": "default", "task_group": "web",
			}
			requireMetric(t, mfs, float64(tt.desired), "nomad_deployment_task_group_desired", tgLabels)
			requireMetric(t, mfs, float64(tt.placed), "nomad_deployment_task_group_placed", tgLabels)
			requireMetric(t, mfs, float64(tt.healthy), "nomad_deployment_task_group_healthy", tgLabels)
			requireMetric(t, mfs, float64(tt.unhealthy), "nomad_deployment_task_group_unhealthy", tgLabels)
		})
	}
}

func TestDeploymentCollector_AutoRevert(t *testing.T) {
	tests := []struct {
		name       string
		tgs        map[string]*nomadapi.DeploymentState
		wantRevert float64
	}{
		{
			name:       "no_task_groups",
			tgs:        nil,
			wantRevert: 0,
		},
		{
			name:       "auto_revert_off",
			tgs:        map[string]*nomadapi.DeploymentState{"web": deployState(3, 3, 3, 0, false)},
			wantRevert: 0,
		},
		{
			name:       "auto_revert_on",
			tgs:        map[string]*nomadapi.DeploymentState{"web": deployState(3, 2, 1, 1, true)},
			wantRevert: 1,
		},
		{
			name: "any_tg_enables_auto_revert",
			tgs: map[string]*nomadapi.DeploymentState{
				"web":    deployState(3, 3, 3, 0, false),
				"worker": deployState(2, 1, 0, 1, true),
			},
			wantRevert: 1,
		},
		{
			name: "all_tgs_no_auto_revert",
			tgs: map[string]*nomadapi.DeploymentState{
				"web":    deployState(3, 3, 3, 0, false),
				"worker": deployState(2, 2, 2, 0, false),
			},
			wantRevert: 0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			d := makeDeployment("dep-1", "myjob", "default", "running", tt.tgs)
			mfs := gatherDeployments(t, newDeploymentMetricCollector(&mockDeploymentLister{deployments: []*nomadapi.Deployment{d}}, nil))

			requireMetric(t, mfs, tt.wantRevert, "nomad_deployment_auto_revert", map[string]string{
				"deployment_id": "dep-1", "job_id": "myjob", "namespace": "default",
			})
		})
	}
}

func TestDeploymentCollector_MultipleTaskGroups(t *testing.T) {
	tgs := map[string]*nomadapi.DeploymentState{
		"api":    deployState(5, 5, 4, 1, false),
		"worker": deployState(3, 2, 2, 0, true),
	}
	d := makeDeployment("dep-1", "platform", "infra", "running", tgs)
	mfs := gatherDeployments(t, newDeploymentMetricCollector(&mockDeploymentLister{deployments: []*nomadapi.Deployment{d}}, nil))

	apiLabels := map[string]string{
		"deployment_id": "dep-1", "job_id": "platform",
		"namespace": "infra", "task_group": "api",
	}
	requireMetric(t, mfs, 5, "nomad_deployment_task_group_desired", apiLabels)
	requireMetric(t, mfs, 5, "nomad_deployment_task_group_placed", apiLabels)
	requireMetric(t, mfs, 4, "nomad_deployment_task_group_healthy", apiLabels)
	requireMetric(t, mfs, 1, "nomad_deployment_task_group_unhealthy", apiLabels)

	workerLabels := map[string]string{
		"deployment_id": "dep-1", "job_id": "platform",
		"namespace": "infra", "task_group": "worker",
	}
	requireMetric(t, mfs, 3, "nomad_deployment_task_group_desired", workerLabels)
	requireMetric(t, mfs, 2, "nomad_deployment_task_group_placed", workerLabels)
	requireMetric(t, mfs, 2, "nomad_deployment_task_group_healthy", workerLabels)
	requireMetric(t, mfs, 0, "nomad_deployment_task_group_unhealthy", workerLabels)

	// worker has AutoRevert=true → deployment-level flag = 1
	requireMetric(t, mfs, 1, "nomad_deployment_auto_revert", map[string]string{
		"deployment_id": "dep-1", "job_id": "platform", "namespace": "infra",
	})
}

func TestDeploymentCollector_MultipleDeployments(t *testing.T) {
	deployments := []*nomadapi.Deployment{
		makeDeployment("dep-a", "jobA", "ns1", "running",
			map[string]*nomadapi.DeploymentState{"web": deployState(3, 3, 3, 0, false)}),
		makeDeployment("dep-b", "jobB", "ns2", "failed",
			map[string]*nomadapi.DeploymentState{"api": deployState(2, 2, 0, 2, true)}),
	}
	mfs := gatherDeployments(t, newDeploymentMetricCollector(&mockDeploymentLister{deployments: deployments}, nil))

	requireMetric(t, mfs, 1, "nomad_deployment_status", map[string]string{
		"deployment_id": "dep-a", "job_id": "jobA", "namespace": "ns1", "status": "running",
	})
	requireMetric(t, mfs, 0, "nomad_deployment_status", map[string]string{
		"deployment_id": "dep-a", "job_id": "jobA", "namespace": "ns1", "status": "failed",
	})
	requireMetric(t, mfs, 1, "nomad_deployment_status", map[string]string{
		"deployment_id": "dep-b", "job_id": "jobB", "namespace": "ns2", "status": "failed",
	})
	requireMetric(t, mfs, 0, "nomad_deployment_auto_revert", map[string]string{
		"deployment_id": "dep-a", "job_id": "jobA", "namespace": "ns1",
	})
	requireMetric(t, mfs, 1, "nomad_deployment_auto_revert", map[string]string{
		"deployment_id": "dep-b", "job_id": "jobB", "namespace": "ns2",
	})
}

func TestDeploymentCollector_EmptyCluster(t *testing.T) {
	mfs := gatherDeployments(t, newDeploymentMetricCollector(&mockDeploymentLister{deployments: nil}, nil))

	for _, name := range []string{
		"nomad_deployment_status",
		"nomad_deployment_task_group_desired",
		"nomad_deployment_task_group_placed",
		"nomad_deployment_task_group_healthy",
		"nomad_deployment_task_group_unhealthy",
		"nomad_deployment_auto_revert",
	} {
		requireAbsent(t, mfs, name)
	}
}
