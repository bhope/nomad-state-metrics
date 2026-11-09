package collector

import (
	"log/slog"
	"net"

	nomadapi "github.com/hashicorp/nomad/api"
	"github.com/prometheus/client_golang/prometheus"
)

// serverLister is the subset of store.NomadStore used by serverCollector.
type serverLister interface {
	ListServers() ([]*nomadapi.AgentMember, map[string]string)
}

var (
	serverInfoDesc = prometheus.NewDesc(
		"nomad_server_info",
		"Static information about a Nomad server.",
		[]string{"name", "region", "datacenter", "address", "version", "status"},
		nil,
	)
	serverLeaderDesc = prometheus.NewDesc(
		"nomad_server_leader",
		"1 if the server is the leader of its region, 0 otherwise.",
		[]string{"name", "region"},
		nil,
	)
)

type serverCollector struct {
	store  serverLister
	logger *slog.Logger
}

func newServerCollector(s serverLister, logger *slog.Logger) *serverCollector {
	return &serverCollector{store: s, logger: logger}
}

func (c *serverCollector) Describe(ch chan<- *prometheus.Desc) {
	ch <- serverInfoDesc
	ch <- serverLeaderDesc
}

func (c *serverCollector) Collect(ch chan<- prometheus.Metric) {
	members, leaders := c.store.ListServers()
	for _, m := range members {
		region := m.Tags["region"]

		ch <- prometheus.MustNewConstMetric(
			serverInfoDesc, prometheus.GaugeValue, 1,
			m.Name, region, m.Tags["dc"], m.Addr, m.Tags["build"], m.Status,
		)

		leader, ok := leaders[region]
		isLeader := ok && leader == net.JoinHostPort(m.Addr, m.Tags["port"])
		ch <- prometheus.MustNewConstMetric(
			serverLeaderDesc, prometheus.GaugeValue, boolFloat(isLeader),
			m.Name, region,
		)
	}
}
