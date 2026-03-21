package collector

import (
	"log/slog"

	nomadapi "github.com/hashicorp/nomad/api"
	"github.com/prometheus/client_golang/prometheus"
)

// nodeLister is the subset of store.NomadStore used by nodeDetailCollector.
type nodeLister interface {
	ListNodes() []*nomadapi.NodeListStub
}

var (
	nodeInfoDesc = prometheus.NewDesc(
		"nomad_node_info",
		"Static information about a Nomad node.",
		[]string{"node_id", "name", "datacenter", "node_class", "version", "status", "scheduling_eligibility", "drain"},
		nil,
	)
	nodeStatusDesc = prometheus.NewDesc(
		"nomad_node_status",
		"Node status as a binary gauge; 1 for the active status, 0 for others.",
		[]string{"node_id", "name", "status"},
		nil,
	)
	nodeDrainDesc = prometheus.NewDesc(
		"nomad_node_drain",
		"1 if the node is draining, 0 otherwise.",
		[]string{"node_id", "name"},
		nil,
	)
	nodeSchedulableDesc = prometheus.NewDesc(
		"nomad_node_schedulable",
		"1 if the node is eligible for scheduling, 0 otherwise.",
		[]string{"node_id", "name"},
		nil,
	)
	nodeAllocCPUDesc = prometheus.NewDesc(
		"nomad_node_allocatable_cpu_mhz",
		"Allocatable CPU capacity of the node in MHz (total minus reserved).",
		[]string{"node_id", "name", "datacenter"},
		nil,
	)
	nodeAllocMemDesc = prometheus.NewDesc(
		"nomad_node_allocatable_memory_bytes",
		"Allocatable memory capacity of the node in bytes (total minus reserved).",
		[]string{"node_id", "name", "datacenter"},
		nil,
	)
)

// nodeStatuses is the fixed set of statuses emitted by nomad_node_status.
var nodeStatuses = []string{"initializing", "ready", "down", "disconnected"}

type nodeDetailCollector struct {
	store  nodeLister
	logger *slog.Logger
}

func newNodeDetailCollector(s nodeLister, logger *slog.Logger) *nodeDetailCollector {
	return &nodeDetailCollector{store: s, logger: logger}
}

func (c *nodeDetailCollector) Describe(ch chan<- *prometheus.Desc) {
	ch <- nodeInfoDesc
	ch <- nodeStatusDesc
	ch <- nodeDrainDesc
	ch <- nodeSchedulableDesc
	ch <- nodeAllocCPUDesc
	ch <- nodeAllocMemDesc
}

func (c *nodeDetailCollector) Collect(ch chan<- prometheus.Metric) {
	for _, n := range c.store.ListNodes() {
		id, name, dc := n.ID, n.Name, n.Datacenter

		ch <- prometheus.MustNewConstMetric(
			nodeInfoDesc, prometheus.GaugeValue, 1,
			id, name, dc, n.NodeClass, n.Version,
			n.Status, n.SchedulingEligibility, boolStr(n.Drain),
		)

		for _, s := range nodeStatuses {
			ch <- prometheus.MustNewConstMetric(
				nodeStatusDesc, prometheus.GaugeValue, boolFloat(n.Status == s),
				id, name, s,
			)
		}

		ch <- prometheus.MustNewConstMetric(
			nodeDrainDesc, prometheus.GaugeValue, boolFloat(n.Drain),
			id, name,
		)

		ch <- prometheus.MustNewConstMetric(
			nodeSchedulableDesc, prometheus.GaugeValue, boolFloat(n.SchedulingEligibility == "eligible"),
			id, name,
		)

		cpu, mem := allocatableResources(n)
		ch <- prometheus.MustNewConstMetric(
			nodeAllocCPUDesc, prometheus.GaugeValue, cpu,
			id, name, dc,
		)
		ch <- prometheus.MustNewConstMetric(
			nodeAllocMemDesc, prometheus.GaugeValue, mem,
			id, name, dc,
		)
	}
}

// allocatableResources returns the allocatable CPU (MHz) and memory (bytes)
// for a node: total NodeResources minus ReservedResources.
func allocatableResources(n *nomadapi.NodeListStub) (cpuMHz float64, memBytes float64) {
	if n.NodeResources != nil {
		cpuMHz = float64(n.NodeResources.Cpu.CpuShares)
		memBytes = float64(n.NodeResources.Memory.MemoryMB) * 1024 * 1024
	}
	if n.ReservedResources != nil {
		cpuMHz -= float64(n.ReservedResources.Cpu.CpuShares)
		memBytes -= float64(n.ReservedResources.Memory.MemoryMB) * 1024 * 1024
	}
	return cpuMHz, memBytes
}
