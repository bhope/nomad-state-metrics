package collector

import (
	"testing"

	nomadapi "github.com/hashicorp/nomad/api"
	dto "github.com/prometheus/client_model/go"
	"github.com/prometheus/client_golang/prometheus"
)

// mockNodeLister implements nodeLister for tests.
type mockNodeLister struct {
	nodes []*nomadapi.NodeListStub
}

func (m *mockNodeLister) ListNodes() []*nomadapi.NodeListStub { return m.nodes }

func gatherNodes(t *testing.T, c prometheus.Collector) []*dto.MetricFamily {
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

func makeNode(id, name, dc, class, version, status, eligibility string, drain bool,
	nodeRes *nomadapi.NodeResources, reservedRes *nomadapi.NodeReservedResources,
) *nomadapi.NodeListStub {
	return &nomadapi.NodeListStub{
		ID:                    id,
		Name:                  name,
		Datacenter:            dc,
		NodeClass:             class,
		Version:               version,
		Status:                status,
		SchedulingEligibility: eligibility,
		Drain:                 drain,
		NodeResources:         nodeRes,
		ReservedResources:     reservedRes,
	}
}

func nodeResources(cpuMHz int64, memMB int64) *nomadapi.NodeResources {
	return &nomadapi.NodeResources{
		Cpu:    nomadapi.NodeCpuResources{CpuShares: cpuMHz},
		Memory: nomadapi.NodeMemoryResources{MemoryMB: memMB},
	}
}

func reservedResources(cpuMHz uint64, memMB uint64) *nomadapi.NodeReservedResources {
	return &nomadapi.NodeReservedResources{
		Cpu:    nomadapi.NodeReservedCpuResources{CpuShares: cpuMHz},
		Memory: nomadapi.NodeReservedMemoryResources{MemoryMB: memMB},
	}
}

func readyNode(id, name, dc string) *nomadapi.NodeListStub {
	return makeNode(id, name, dc, "", "1.9.0", "ready", "eligible", false,
		nodeResources(4000, 8192), nil)
}

// ── test cases ────────────────────────────────────────────────────────────────

func TestNodeCollector_Info(t *testing.T) {
	n := makeNode("node-1", "web-01", "dc1", "high-memory", "1.9.0",
		"ready", "eligible", false, nodeResources(4000, 8192), nil)
	mfs := gatherNodes(t, newNodeDetailCollector(&mockNodeLister{nodes: []*nomadapi.NodeListStub{n}}, nil))

	requireMetric(t, mfs, 1, "nomad_node_info", map[string]string{
		"node_id": "node-1", "name": "web-01", "datacenter": "dc1",
		"node_class": "high-memory", "version": "1.9.0",
		"status": "ready", "scheduling_eligibility": "eligible", "drain": "false",
	})
}

func TestNodeCollector_InfoDraining(t *testing.T) {
	n := makeNode("node-2", "worker-01", "dc2", "", "1.8.0",
		"ready", "ineligible", true, nodeResources(2000, 4096), nil)
	mfs := gatherNodes(t, newNodeDetailCollector(&mockNodeLister{nodes: []*nomadapi.NodeListStub{n}}, nil))

	requireMetric(t, mfs, 1, "nomad_node_info", map[string]string{
		"node_id": "node-2", "name": "worker-01", "datacenter": "dc2",
		"node_class": "", "version": "1.8.0",
		"status": "ready", "scheduling_eligibility": "ineligible", "drain": "true",
	})
}

func TestNodeCollector_Status(t *testing.T) {
	tests := []struct {
		status string
	}{
		{"initializing"},
		{"ready"},
		{"down"},
		{"disconnected"},
	}

	for _, tt := range tests {
		t.Run(tt.status, func(t *testing.T) {
			n := makeNode("node-1", "web-01", "dc1", "", "1.9.0",
				tt.status, "eligible", false, nil, nil)
			mfs := gatherNodes(t, newNodeDetailCollector(&mockNodeLister{nodes: []*nomadapi.NodeListStub{n}}, nil))

			for _, s := range nodeStatuses {
				requireMetric(t, mfs, boolFloat(s == tt.status), "nomad_node_status", map[string]string{
					"node_id": "node-1", "name": "web-01", "status": s,
				})
			}
		})
	}
}

func TestNodeCollector_Drain(t *testing.T) {
	tests := []struct {
		name      string
		drain     bool
		wantDrain float64
	}{
		{"not_draining", false, 0},
		{"draining", true, 1},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			n := makeNode("node-1", "web-01", "dc1", "", "1.9.0",
				"ready", "eligible", tt.drain, nil, nil)
			mfs := gatherNodes(t, newNodeDetailCollector(&mockNodeLister{nodes: []*nomadapi.NodeListStub{n}}, nil))

			requireMetric(t, mfs, tt.wantDrain, "nomad_node_drain", map[string]string{
				"node_id": "node-1", "name": "web-01",
			})
		})
	}
}

func TestNodeCollector_Schedulable(t *testing.T) {
	tests := []struct {
		name          string
		eligibility   string
		wantSchedulable float64
	}{
		{"eligible", "eligible", 1},
		{"ineligible", "ineligible", 0},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			n := makeNode("node-1", "web-01", "dc1", "", "1.9.0",
				"ready", tt.eligibility, false, nil, nil)
			mfs := gatherNodes(t, newNodeDetailCollector(&mockNodeLister{nodes: []*nomadapi.NodeListStub{n}}, nil))

			requireMetric(t, mfs, tt.wantSchedulable, "nomad_node_schedulable", map[string]string{
				"node_id": "node-1", "name": "web-01",
			})
		})
	}
}

func TestNodeCollector_AllocatableResources(t *testing.T) {
	tests := []struct {
		name        string
		nodeRes     *nomadapi.NodeResources
		reservedRes *nomadapi.NodeReservedResources
		wantCPU     float64
		wantMemBytes float64
	}{
		{
			name:         "no_resources",
			nodeRes:      nil,
			reservedRes:  nil,
			wantCPU:      0,
			wantMemBytes: 0,
		},
		{
			name:         "resources_no_reserved",
			nodeRes:      nodeResources(4000, 8192),
			reservedRes:  nil,
			wantCPU:      4000,
			wantMemBytes: 8192 * 1024 * 1024,
		},
		{
			name:         "resources_with_reserved",
			nodeRes:      nodeResources(4000, 8192),
			reservedRes:  reservedResources(500, 1024),
			wantCPU:      3500,
			wantMemBytes: 7168 * 1024 * 1024,
		},
		{
			name:         "fully_reserved",
			nodeRes:      nodeResources(2000, 4096),
			reservedRes:  reservedResources(2000, 4096),
			wantCPU:      0,
			wantMemBytes: 0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			n := makeNode("node-1", "web-01", "dc1", "", "1.9.0",
				"ready", "eligible", false, tt.nodeRes, tt.reservedRes)
			mfs := gatherNodes(t, newNodeDetailCollector(&mockNodeLister{nodes: []*nomadapi.NodeListStub{n}}, nil))

			labels := map[string]string{"node_id": "node-1", "name": "web-01", "datacenter": "dc1"}
			requireMetric(t, mfs, tt.wantCPU, "nomad_node_allocatable_cpu_mhz", labels)
			requireMetric(t, mfs, tt.wantMemBytes, "nomad_node_allocatable_memory_bytes", labels)
		})
	}
}

func TestNodeCollector_MultipleNodes(t *testing.T) {
	nodes := []*nomadapi.NodeListStub{
		readyNode("node-1", "web-01", "dc1"),
		makeNode("node-2", "worker-01", "dc2", "", "1.9.0", "down", "ineligible", false,
			nodeResources(8000, 16384), reservedResources(1000, 2048)),
	}
	mfs := gatherNodes(t, newNodeDetailCollector(&mockNodeLister{nodes: nodes}, nil))

	// node-1: ready, eligible
	requireMetric(t, mfs, 1, "nomad_node_status", map[string]string{
		"node_id": "node-1", "name": "web-01", "status": "ready",
	})
	requireMetric(t, mfs, 0, "nomad_node_status", map[string]string{
		"node_id": "node-1", "name": "web-01", "status": "down",
	})
	requireMetric(t, mfs, 1, "nomad_node_schedulable", map[string]string{
		"node_id": "node-1", "name": "web-01",
	})

	// node-2: down, ineligible, with reserved resources
	requireMetric(t, mfs, 1, "nomad_node_status", map[string]string{
		"node_id": "node-2", "name": "worker-01", "status": "down",
	})
	requireMetric(t, mfs, 0, "nomad_node_schedulable", map[string]string{
		"node_id": "node-2", "name": "worker-01",
	})
	requireMetric(t, mfs, 7000, "nomad_node_allocatable_cpu_mhz", map[string]string{
		"node_id": "node-2", "name": "worker-01", "datacenter": "dc2",
	})
	requireMetric(t, mfs, float64(14336)*1024*1024, "nomad_node_allocatable_memory_bytes", map[string]string{
		"node_id": "node-2", "name": "worker-01", "datacenter": "dc2",
	})
}

func TestNodeCollector_EmptyCluster(t *testing.T) {
	mfs := gatherNodes(t, newNodeDetailCollector(&mockNodeLister{nodes: nil}, nil))

	for _, name := range []string{
		"nomad_node_info", "nomad_node_status", "nomad_node_drain",
		"nomad_node_schedulable", "nomad_node_allocatable_cpu_mhz",
		"nomad_node_allocatable_memory_bytes",
	} {
		requireAbsent(t, mfs, name)
	}
}
