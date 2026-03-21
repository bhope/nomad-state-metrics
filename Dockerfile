# syntax=docker/dockerfile:1
FROM golang:1.25-alpine AS builder

ARG VERSION=dev
WORKDIR /src

COPY go.mod go.sum ./
RUN go mod download

COPY . .
RUN CGO_ENABLED=0 go build \
      -ldflags "-s -w -X main.version=${VERSION}" \
      -o /bin/nomad-state-metrics \
      ./cmd/nomad-state-metrics

# ---

FROM scratch

COPY --from=builder /etc/ssl/certs/ca-certificates.crt /etc/ssl/certs/
COPY --from=builder /bin/nomad-state-metrics /nomad-state-metrics

EXPOSE 9290

ENTRYPOINT ["/nomad-state-metrics"]
