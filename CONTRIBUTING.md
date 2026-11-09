# Contributing

Thanks for your interest in improving nomad-state-metrics.

## Getting Started

You need Go (see the version in `go.mod`) and optionally [golangci-lint](https://golangci-lint.run/).

```sh
make build
make test
make lint
```

## Making Changes

1. Open an issue first for larger changes so we can agree on the approach.
2. Fork the repo and create a branch from `main`.
3. Keep collector logic in `internal/collector/` and API polling in `internal/store/`.
4. Make sure `make test` and `make lint` pass.
5. Open a pull request describing what changed and why.

## Adding a Metric

1. Add a `prometheus.Desc` var for it.
2. Send it from `Describe` and emit it in `Collect`.
3. Add a unit test checking the metric name, labels and value.
4. Add it to the metrics reference in the README.

## Reporting Security Issues

See [SECURITY.md](SECURITY.md).
