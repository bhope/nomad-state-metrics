package collector

import (
	"testing"

	nomadapi "github.com/hashicorp/nomad/api"
)

type mockServerLister struct {
	members []*nomadapi.AgentMember
	leaders map[string]string
}

func (m *mockServerLister) ListServers() ([]*nomadapi.AgentMember, map[string]string) {
	return m.members, m.leaders
}

func makeServer(name, addr, region, dc, status string) *nomadapi.AgentMember {
	return &nomadapi.AgentMember{
		Name:   name + "." + region,
		Addr:   addr,
		Status: status,
		Tags: map[string]string{
			"region": region,
			"dc":     dc,
			"port":   "4647",
			"build":  "1.11.0",
		},
	}
}

func TestServerCollector_Info(t *testing.T) {
	s := makeServer("server-1", "10.0.0.1", "global", "dc1", "alive")
	mfs := gatherNodes(t, newServerCollector(&mockServerLister{members: []*nomadapi.AgentMember{s}}, nil))

	requireMetric(t, mfs, 1, "nomad_server_info", map[string]string{
		"name": "server-1.global", "region": "global", "datacenter": "dc1",
		"address": "10.0.0.1", "version": "1.11.0", "status": "alive",
	})
}

func TestServerCollector_Leader(t *testing.T) {
	ls := &mockServerLister{
		members: []*nomadapi.AgentMember{
			makeServer("server-1", "10.0.0.1", "global", "dc1", "alive"),
			makeServer("server-2", "10.0.0.2", "global", "dc1", "alive"),
			makeServer("server-3", "10.1.0.1", "eu", "dc2", "alive"),
			makeServer("server-4", "10.1.0.2", "eu", "dc2", "alive"),
		},
		leaders: map[string]string{
			"global": "10.0.0.2:4647",
			"eu":     "10.1.0.1:4647",
		},
	}
	mfs := gatherNodes(t, newServerCollector(ls, nil))

	tests := []struct {
		name, region string
		want         float64
	}{
		{"server-1.global", "global", 0},
		{"server-2.global", "global", 1},
		{"server-3.eu", "eu", 1},
		{"server-4.eu", "eu", 0},
	}
	for _, tt := range tests {
		requireMetric(t, mfs, tt.want, "nomad_server_leader", map[string]string{
			"name": tt.name, "region": tt.region,
		})
	}
}

func TestServerCollector_UnknownLeader(t *testing.T) {
	ls := &mockServerLister{
		members: []*nomadapi.AgentMember{makeServer("server-1", "10.0.0.1", "global", "dc1", "alive")},
	}
	mfs := gatherNodes(t, newServerCollector(ls, nil))

	requireMetric(t, mfs, 0, "nomad_server_leader", map[string]string{
		"name": "server-1.global", "region": "global",
	})
}

func TestServerCollector_IPv6Leader(t *testing.T) {
	ls := &mockServerLister{
		members: []*nomadapi.AgentMember{makeServer("server-1", "fd00::1", "global", "dc1", "alive")},
		leaders: map[string]string{"global": "[fd00::1]:4647"},
	}
	mfs := gatherNodes(t, newServerCollector(ls, nil))

	requireMetric(t, mfs, 1, "nomad_server_leader", map[string]string{
		"name": "server-1.global", "region": "global",
	})
}
