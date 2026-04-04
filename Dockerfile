FROM gcr.io/distroless/static-debian12:nonroot

COPY nomad-state-metrics /nomad-state-metrics

EXPOSE 9441 9442

ENTRYPOINT ["/nomad-state-metrics"]
