package collector

import (
	"testing"

	nomadapi "github.com/hashicorp/nomad/api"
	"github.com/prometheus/client_golang/prometheus"
	dto "github.com/prometheus/client_model/go"
)

// mockEvaluationLister implements evaluationLister for tests.
type mockEvaluationLister struct {
	evals []*nomadapi.Evaluation
}

func (m *mockEvaluationLister) ListEvaluations() []*nomadapi.Evaluation { return m.evals }

func gatherEvals(t *testing.T, c prometheus.Collector) []*dto.MetricFamily {
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

func makeEval(id, jobID, ns, status string, failedTGs map[string]*nomadapi.AllocationMetric) *nomadapi.Evaluation {
	return &nomadapi.Evaluation{
		ID:             id,
		JobID:          jobID,
		Namespace:      ns,
		Status:         status,
		FailedTGAllocs: failedTGs,
	}
}

func failedTG(coalescedFailures int) *nomadapi.AllocationMetric {
	return &nomadapi.AllocationMetric{CoalescedFailures: coalescedFailures}
}

// ── test cases ────────────────────────────────────────────────────────────────

func TestEvalCollector_Status(t *testing.T) {
	tests := []struct {
		status string
	}{
		{"blocked"},
		{"pending"},
		{"complete"},
		{"failed"},
		{"canceled"},
	}

	for _, tt := range tests {
		t.Run(tt.status, func(t *testing.T) {
			e := makeEval("eval-1", "myjob", "default", tt.status, nil)
			mfs := gatherEvals(t, newEvalMetricCollector(&mockEvaluationLister{evals: []*nomadapi.Evaluation{e}}, nil))

			for _, s := range evalStatuses {
				requireMetric(t, mfs, boolFloat(s == tt.status), "nomad_evaluation_status", map[string]string{
					"eval_id": "eval-1", "job_id": "myjob",
					"namespace": "default", "status": s,
				})
			}
		})
	}
}

func TestEvalCollector_FailedTGAllocs(t *testing.T) {
	tests := []struct {
		name      string
		failedTGs map[string]*nomadapi.AllocationMetric
		taskGroup string
		wantCount float64
	}{
		{
			name:      "single_failure_no_coalesced",
			failedTGs: map[string]*nomadapi.AllocationMetric{"web": failedTG(0)},
			taskGroup: "web",
			wantCount: 1,
		},
		{
			name:      "single_failure_with_coalesced",
			failedTGs: map[string]*nomadapi.AllocationMetric{"web": failedTG(4)},
			taskGroup: "web",
			wantCount: 5,
		},
		{
			name:      "worker_group_coalesced",
			failedTGs: map[string]*nomadapi.AllocationMetric{"worker": failedTG(9)},
			taskGroup: "worker",
			wantCount: 10,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			e := makeEval("eval-1", "myjob", "default", "failed", tt.failedTGs)
			mfs := gatherEvals(t, newEvalMetricCollector(&mockEvaluationLister{evals: []*nomadapi.Evaluation{e}}, nil))

			requireMetric(t, mfs, tt.wantCount, "nomad_evaluation_failed_tg_allocs", map[string]string{
				"eval_id": "eval-1", "job_id": "myjob",
				"namespace": "default", "task_group": tt.taskGroup,
			})
		})
	}
}

func TestEvalCollector_NoFailedTGAllocs(t *testing.T) {
	tests := []struct {
		name      string
		failedTGs map[string]*nomadapi.AllocationMetric
	}{
		{"nil_map", nil},
		{"empty_map", map[string]*nomadapi.AllocationMetric{}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			e := makeEval("eval-1", "myjob", "default", "complete", tt.failedTGs)
			mfs := gatherEvals(t, newEvalMetricCollector(&mockEvaluationLister{evals: []*nomadapi.Evaluation{e}}, nil))

			requireAbsent(t, mfs, "nomad_evaluation_failed_tg_allocs")
		})
	}
}

func TestEvalCollector_MultipleFailedTaskGroups(t *testing.T) {
	failedTGs := map[string]*nomadapi.AllocationMetric{
		"api":    failedTG(0),
		"worker": failedTG(2),
		"cache":  failedTG(1),
	}
	e := makeEval("eval-1", "platform", "infra", "failed", failedTGs)
	mfs := gatherEvals(t, newEvalMetricCollector(&mockEvaluationLister{evals: []*nomadapi.Evaluation{e}}, nil))

	for tg, want := range map[string]float64{"api": 1, "worker": 3, "cache": 2} {
		requireMetric(t, mfs, want, "nomad_evaluation_failed_tg_allocs", map[string]string{
			"eval_id": "eval-1", "job_id": "platform",
			"namespace": "infra", "task_group": tg,
		})
	}
}

func TestEvalCollector_MultipleEvals(t *testing.T) {
	evals := []*nomadapi.Evaluation{
		makeEval("eval-a", "jobA", "ns1", "complete", nil),
		makeEval("eval-b", "jobB", "ns2", "failed",
			map[string]*nomadapi.AllocationMetric{"web": failedTG(1)}),
	}
	mfs := gatherEvals(t, newEvalMetricCollector(&mockEvaluationLister{evals: evals}, nil))

	requireMetric(t, mfs, 1, "nomad_evaluation_status", map[string]string{
		"eval_id": "eval-a", "job_id": "jobA", "namespace": "ns1", "status": "complete",
	})
	requireMetric(t, mfs, 0, "nomad_evaluation_status", map[string]string{
		"eval_id": "eval-a", "job_id": "jobA", "namespace": "ns1", "status": "failed",
	})
	requireMetric(t, mfs, 1, "nomad_evaluation_status", map[string]string{
		"eval_id": "eval-b", "job_id": "jobB", "namespace": "ns2", "status": "failed",
	})
	requireMetric(t, mfs, 2, "nomad_evaluation_failed_tg_allocs", map[string]string{
		"eval_id": "eval-b", "job_id": "jobB", "namespace": "ns2", "task_group": "web",
	})
}

func TestEvalCollector_EmptyCluster(t *testing.T) {
	mfs := gatherEvals(t, newEvalMetricCollector(&mockEvaluationLister{evals: nil}, nil))

	requireAbsent(t, mfs, "nomad_evaluation_status")
	requireAbsent(t, mfs, "nomad_evaluation_failed_tg_allocs")
}
