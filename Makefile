BINARY     := nomad-state-metrics
MODULE     := github.com/bhope/nomad-state-metrics
CMD        := ./cmd/nomad-state-metrics
IMAGE      ?= ghcr.io/bhope/nomad-state-metrics
VERSION    ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo dev)
LDFLAGS    := -s -w -X main.version=$(VERSION)

.PHONY: all build test lint fmt vet tidy docker docker-push clean

all: build

build:
	go build -ldflags "$(LDFLAGS)" -o bin/$(BINARY) $(CMD)

test:
	go test -race -count=1 ./...

lint:
	golangci-lint run ./...

fmt:
	gofmt -w -s .

vet:
	go vet ./...

tidy:
	go mod tidy

docker:
	docker build \
		--build-arg VERSION=$(VERSION) \
		-t $(IMAGE):$(VERSION) \
		-t $(IMAGE):latest \
		.

docker-push: docker
	docker push $(IMAGE):$(VERSION)
	docker push $(IMAGE):latest

clean:
	rm -rf bin/
