# nomad-state-metrics

Prometheus exporter that generates metrics about the state of [Nomad](https://www.nomadproject.io/) objects. Inspired by [kube-state-metrics](https://github.com/kubernetes/kube-state-metrics) for Kubernetes.

---

## Overview

Nomad's observability story is split across three layers:

| Layer | Tool | What it covers |
|---|---|---|
| 1 | [node_exporter](https://github.com/prometheus/node_exporter) | Host-level metrics: CPU, memory, disk, network |
| 2 | Nomad built-in [`/v1/metrics`](https://developer.hashicorp.com/nomad/docs/reference/metrics) | Nomad process internals: scheduler latency, RPC rates, Raft health |
| 3 | **nomad-state-metrics** | Workload object state: job status, allocation health, node readiness |

**The gap this project fills:** neither layer 1 nor layer 2 answers questions like "how many allocations are currently failed?", "is this job dead?", or "is this node draining?". nomad-state-metrics polls the Nomad API on a configurable interval, caches the results, and exposes them as Prometheus gauges so your alerting rules can answer these questions without writing custom scripts.

The design is directly inspired by [kube-state-metrics](https://github.com/kubernetes/kube-state-metrics)' philosophy: expose _object state_ as metrics, keep them separate from process and host metrics, and serve them on a dedicated port so scrape configs stay clean.

---

## Quick Start

### Binary

```sh
go install github.com/bhope/nomad-state-metrics/cmd/nomad-state-metrics@latest

nomad-state-metrics \
  -nomad-address http://nomad.example.com:4646 \
  -poll-interval 30s \
  -port 9441 \
  -telemetry-port 9442
```

Metrics are available at `http://localhost:9441/metrics`.
Self-metrics and a health check are available at `http://localhost:9442/metrics` and `http://localhost:9442/healthz`.

### Docker

```sh
docker run --rm \
  -e NOMAD_ADDR=http://nomad.example.com:4646 \
  -p 9441:9441 \
  -p 9442:9442 \
  ghcr.io/bhope/nomad-state-metrics:latest
```

---

## Configuration

All configuration is via CLI flags. Environment variables supported by the Nomad Go SDK (`NOMAD_ADDR`, `NOMAD_TOKEN`, `NOMAD_NAMESPACE`, `NOMAD_TLS_*`) are also respected.

| Flag | Default | Description |
|---|---|---|
| `-nomad-address` | `http://localhost:4646` | Nomad API address |
| `-port` | `9441` | Port to serve Nomad state metrics on (`/metrics`) |
| `-telemetry-port` | `9442` | Port to serve self-metrics (`/metrics`) and health check (`/healthz`) on |
| `-poll-interval` | `30s` | Interval between Nomad API polls. Accepts Go duration strings: `10s`, `1m`, etc. |
| `-log-level` | `info` | Log level. One of: `debug`, `info`, `warn`, `error` |

### Two-port design

nomad-state-metrics intentionally separates concerns across two ports:

- **`:9441/metrics`** — Nomad object state metrics only. Add this as a Prometheus scrape target.
- **`:9442/metrics`** — Go runtime and process metrics for the exporter itself (goroutines, GC, memory).
- **`:9442/healthz`** — Returns HTTP 200 when the process is alive. Use for liveness probes.

---

## Metrics Reference

All metrics are `Gauge` type. Status metrics use a binary encoding: for each possible status value, a separate time series is emitted with value `1` if that status is active and `0` otherwise. This makes alerting rules straightforward (`== 1`) and avoids string-valued labels.

### Jobs

| Metric Name | Labels | Description |
|---|---|---|
| `nomad_job_info` | `job_id`, `namespace`, `job_type`, `status`, `priority` | Static job information; always `1`. Use label filters to identify jobs. |
| `nomad_job_status` | `job_id`, `namespace`, `status` | Binary gauge for job status. `status` is one of: `running`, `pending`, `dead`. |
| `nomad_job_task_groups_desired` | `job_id`, `namespace`, `task_group` | Number of desired allocations for a task group (queued + starting + running). |
| `nomad_job_task_groups_running` | `job_id`, `namespace`, `task_group` | Number of currently running allocations for a task group. |
| `nomad_job_version` | `job_id`, `namespace` | `JobModifyIndex` of the job, used as a monotonic version proxy. |
| `nomad_job_is_periodic` | `job_id`, `namespace` | `1` if the job is periodic, `0` otherwise. |
| `nomad_job_is_parameterized` | `job_id`, `namespace` | `1` if the job is parameterized, `0` otherwise. |

### Allocations

| Metric Name | Labels | Description |
|---|---|---|
| `nomad_allocation_status` | `job_id`, `namespace`, `task_group`, `alloc_id`, `node_id`, `status` | Binary gauge for allocation client status. `status` is one of: `pending`, `running`, `complete`, `failed`, `lost`. |
| `nomad_allocation_desired_status` | `job_id`, `namespace`, `task_group`, `alloc_id`, `desired_status` | Binary gauge for allocation desired status. `desired_status` is one of: `run`, `stop`, `evict`. |
| `nomad_allocation_health` | `job_id`, `namespace`, `task_group`, `alloc_id` | Deployment health: `1` = healthy, `0` = unhealthy, `-1` = unknown (not yet evaluated). |
| `nomad_allocation_restart_count` | `job_id`, `namespace`, `task_group`, `alloc_id`, `task_name` | Total number of restarts for a task within an allocation. |
| `nomad_allocation_created_timestamp` | `job_id`, `namespace`, `task_group`, `alloc_id` | Unix timestamp (seconds) of when the allocation was created. |
| `nomad_allocation_task_state` | `job_id`, `namespace`, `task_group`, `alloc_id`, `task_name`, `state` | Binary gauge for individual task state within an allocation. `state` is one of: `pending`, `running`, `dead`. |

### Nodes

| Metric Name | Labels | Description |
|---|---|---|
| `nomad_node_info` | `node_id`, `name`, `datacenter`, `node_class`, `version`, `status`, `scheduling_eligibility`, `drain` | Static node information; always `1`. Use label filters to identify nodes. |
| `nomad_node_status` | `node_id`, `name`, `status` | Binary gauge for node status. `status` is one of: `initializing`, `ready`, `down`, `disconnected`. |
| `nomad_node_drain` | `node_id`, `name` | `1` if the node is draining, `0` otherwise. |
| `nomad_node_schedulable` | `node_id`, `name` | `1` if the node is eligible for scheduling, `0` otherwise. |
| `nomad_node_allocatable_cpu_mhz` | `node_id`, `name`, `datacenter` | Allocatable CPU capacity in MHz (total node CPU minus reserved CPU). |
| `nomad_node_allocatable_memory_bytes` | `node_id`, `name`, `datacenter` | Allocatable memory capacity in bytes (total node memory minus reserved memory). |

### Deployments

| Metric Name | Labels | Description |
|---|---|---|
| `nomad_deployment_status` | `deployment_id`, `job_id`, `namespace`, `status` | Binary gauge for deployment status. `status` is one of: `running`, `successful`, `cancelled`, `failed`. |
| `nomad_deployment_task_group_desired` | `deployment_id`, `job_id`, `namespace`, `task_group` | Desired number of allocations for a task group in a deployment. |
| `nomad_deployment_task_group_placed` | `deployment_id`, `job_id`, `namespace`, `task_group` | Number of placed allocations for a task group in a deployment. |
| `nomad_deployment_task_group_healthy` | `deployment_id`, `job_id`, `namespace`, `task_group` | Number of healthy allocations for a task group in a deployment. |
| `nomad_deployment_task_group_unhealthy` | `deployment_id`, `job_id`, `namespace`, `task_group` | Number of unhealthy allocations for a task group in a deployment. |
| `nomad_deployment_auto_revert` | `deployment_id`, `job_id`, `namespace` | `1` if any task group in the deployment has auto-revert enabled, `0` otherwise. |

### Evaluations

| Metric Name | Labels | Description |
|---|---|---|
| `nomad_evaluation_status` | `eval_id`, `job_id`, `namespace`, `status` | Binary gauge for evaluation status. `status` is one of: `blocked`, `pending`, `complete`, `failed`, `canceled`. |
| `nomad_evaluation_failed_tg_allocs` | `eval_id`, `job_id`, `namespace`, `task_group` | Number of failed allocation attempts for a task group in an evaluation (`CoalescedFailures + 1`). Only emitted when a task group has failures. |

---

## Example Prometheus Alerting Rules

See [`examples/alerting-rules.yml`](examples/alerting-rules.yml) for a ready-to-use alerting rule file.

| Rule | Fires when | Description |
|---|---|---|
| `NomadJobDead` | `nomad_job_status{status="dead"} == 1` | A job has reached the `dead` status and is no longer running or being rescheduled. |
| `NomadAllocationFailed` | `nomad_allocation_status{status="failed"} == 1` | An allocation has failed on its client node. Sustained failures indicate a problem that Nomad's rescheduler cannot self-heal. |
| `NomadAllocationRestart` | `nomad_allocation_restart_count > 5` | A task within an allocation has restarted more than 5 times, indicating a crash-looping task. |

---

## Deploying on Nomad

See [`examples/nomad-job.hcl`](examples/nomad-job.hcl) for a Nomad job spec that runs nomad-state-metrics as a system job on your cluster.

Using the `system` scheduler ensures one exporter instance per client node, which works well when Prometheus uses node-level service discovery. Switch to `service` with `count = 1` if you prefer a single centralized exporter instance.

The job exposes:
- Port `9441` — scrape target for Prometheus (Nomad state metrics)
- Port `9442` — health check endpoint (`/healthz`) for Nomad service checks

---

## Contributing

1. Fork the repository and create a feature branch.
2. Make your changes. Keep collector logic in `internal/collector/` and API polling in `internal/store/`.
3. Run `go test ./...` before submitting — all tests must pass.
4. Run `go vet ./...` and ensure there are no linter warnings.
5. Open a pull request with a clear description of what the change does and why.

For new metrics: add the `prometheus.Desc` var, emit it in `Collect`, register it in `Describe`, and add a unit test that verifies the metric name, labels, and value.

---

## License

Apache License 2.0. See [LICENSE](LICENSE).
