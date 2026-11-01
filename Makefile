MODULE      := eks-upgrader
BINARY      := eks-upgrader
VERSION     ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo dev)
COMMIT      ?= $(shell git rev-parse --short HEAD 2>/dev/null || echo none)
DATE        := $(shell date -u +%Y-%m-%dT%H:%M:%SZ)
LDFLAGS     := -s -w \
	-X $(MODULE)/internal/cli.Version=$(VERSION) \
	-X $(MODULE)/internal/cli.Commit=$(COMMIT) \
	-X $(MODULE)/internal/cli.Date=$(DATE)

.PHONY: build
build:
	CGO_ENABLED=0 go build -ldflags '$(LDFLAGS)' -o bin/$(BINARY) ./cmd/eks-upgrader

.PHONY: test
test:
	go test ./... -race -count=1

.PHONY: coverage
coverage:
	go test ./... -race -count=1 -coverprofile=coverage.out
	go tool cover -func=coverage.out

.PHONY: vet
vet:
	go vet ./...

.PHONY: fmt
fmt:
	gofmt -l -w .

.PHONY: lint
lint: vet
	@command -v golangci-lint >/dev/null 2>&1 && golangci-lint run ./... || echo "golangci-lint not installed, skipping"

.PHONY: tidy
tidy:
	go mod tidy

.PHONY: docker
docker:
	docker build --build-arg VERSION=$(VERSION) --build-arg COMMIT=$(COMMIT) -t eks-upgrader:$(VERSION) .

.PHONY: clean
clean:
	rm -rf bin coverage.out

.PHONY: install-tools
install-tools:
	./scripts/install-tools.sh

.PHONY: smoke-test
smoke-test: build
	./scripts/smoke-test.sh
