FROM gcr.io/distroless/static-debian12:nonroot

COPY nomad-state-metrics /nomad-state-metrics

EXPOSE 9290

ENTRYPOINT ["/nomad-state-metrics"]
