// Package store provides a polling cache for Nomad API resources,
// decoupling Prometheus scrape frequency from Nomad API poll frequency.
package store

import (
	"context"
	"log/slog"
	"sync"
	"time"

	nomadapi "github.com/hashicorp/nomad/api"
)

// NomadStore periodically polls the Nomad API and caches the results
// so that Prometheus scrapes read from memory rather than hitting Nomad directly.
type NomadStore struct {
	client          *nomadapi.Client
	pollInterval    time.Duration
	logger          *slog.Logger
	jobStatusFilter map[string]struct{}

	mu          sync.RWMutex
	jobs        []*nomadapi.JobListStub
	allocs      []*nomadapi.AllocationListStub
	nodes       []*nomadapi.NodeListStub
	deployments []*nomadapi.Deployment
	evaluations []*nomadapi.Evaluation
}

// New returns a NomadStore that will poll the given client on the given interval.
func New(client *nomadapi.Client, pollInterval time.Duration, logger *slog.Logger, jobStatusFilter map[string]struct{}) *NomadStore {
	return &NomadStore{
		client:          client,
		pollInterval:    pollInterval,
		logger:          logger,
		jobStatusFilter: jobStatusFilter,
	}
}

// Run blocks, polling Nomad on each tick until ctx is cancelled.
// It performs an initial poll immediately before entering the ticker loop.
func (s *NomadStore) Run(ctx context.Context) {
	s.poll()

	ticker := time.NewTicker(s.pollInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			s.poll()
		case <-ctx.Done():
			return
		}
	}
}

func (s *NomadStore) poll() {
	q := &nomadapi.QueryOptions{Namespace: "*"}

	if jobs, _, err := s.client.Jobs().List(q); err != nil {
		s.logger.Error("poll: failed to list jobs", "error", err)
	} else {
		if len(s.jobStatusFilter) > 0 {
			filtered := jobs[:0]
			for _, j := range jobs {
				if _, ok := s.jobStatusFilter[j.Status]; ok {
					filtered = append(filtered, j)
				}
			}
			jobs = filtered
		}
		s.mu.Lock()
		s.jobs = jobs
		s.mu.Unlock()
	}

	if allocs, _, err := s.client.Allocations().List(q); err != nil {
		s.logger.Error("poll: failed to list allocations", "error", err)
	} else {
		s.mu.Lock()
		s.allocs = allocs
		s.mu.Unlock()
	}

	if nodes, _, err := s.client.Nodes().List(nil); err != nil {
		s.logger.Error("poll: failed to list nodes", "error", err)
	} else {
		s.mu.Lock()
		s.nodes = nodes
		s.mu.Unlock()
	}

	if deploys, _, err := s.client.Deployments().List(q); err != nil {
		s.logger.Error("poll: failed to list deployments", "error", err)
	} else {
		s.mu.Lock()
		s.deployments = deploys
		s.mu.Unlock()
	}

	if evals, _, err := s.client.Evaluations().List(q); err != nil {
		s.logger.Error("poll: failed to list evaluations", "error", err)
	} else {
		s.mu.Lock()
		s.evaluations = evals
		s.mu.Unlock()
	}
}

// ListJobs returns the most recently cached job list.
func (s *NomadStore) ListJobs() []*nomadapi.JobListStub {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.jobs
}

// ListAllocations returns the most recently cached allocation list.
func (s *NomadStore) ListAllocations() []*nomadapi.AllocationListStub {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.allocs
}

// ListNodes returns the most recently cached node list.
func (s *NomadStore) ListNodes() []*nomadapi.NodeListStub {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.nodes
}

// ListDeployments returns the most recently cached deployment list.
func (s *NomadStore) ListDeployments() []*nomadapi.Deployment {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.deployments
}

// ListEvaluations returns the most recently cached evaluation list.
func (s *NomadStore) ListEvaluations() []*nomadapi.Evaluation {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.evaluations
}
