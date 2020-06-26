.PHONY: clean test check build.local build.linux build.osx build.docker build.push

BINARY          ?= kube-metrics-adapter
VERSION         ?= $(shell git describe --tags --always --dirty)
IMAGE           ?= registry-write.opensource.zalan.do/teapot/$(BINARY)
TAG             ?= $(VERSION)
SOURCES         = $(shell find . -name '*.go')
DOCKERFILE      ?= Dockerfile
GOPKGS          = $(shell go list ./...)
GO_OPENAPI_GEN = ./build/openapi-gen
OPENAPI_GEN     = pkg/apiserver/generated/openapi/zz_generated.openapi.go
BUILD_FLAGS     ?= -v
LDFLAGS         ?= -X main.version=$(VERSION) -w -s

default: build.local

clean:
	rm -rf build
	rm -rf $(OPENAPI_GEN)

test:
	go test -v $(GOPKGS)

check:
	go mod download
	golangci-lint run --timeout=2m ./...

$(GO_OPENAPI_GEN):
	mkdir -p build
	GOBIN=$(shell pwd)/build go install k8s.io/kube-openapi/cmd/openapi-gen

$(OPENAPI_GEN): $(GO_OPENAPI_GEN)
	$(GO_OPENAPI_GEN) -o . --go-header-file hack/boilerplate.go.txt --logtostderr -i k8s.io/metrics/pkg/apis/custom_metrics,k8s.io/metrics/pkg/apis/custom_metrics/v1beta1,k8s.io/metrics/pkg/apis/custom_metrics/v1beta2,k8s.io/metrics/pkg/apis/external_metrics,k8s.io/metrics/pkg/apis/external_metrics/v1beta1,k8s.io/apimachinery/pkg/apis/meta/v1,k8s.io/apimachinery/pkg/api/resource,k8s.io/apimachinery/pkg/version,k8s.io/api/core/v1 -p pkg/apiserver/generated/openapi -O zz_generated.openapi -r /dev/null

build.local: build/$(BINARY)
build.linux: build/linux/$(BINARY)
build.osx: build/osx/$(BINARY)

build/$(BINARY): go.mod $(OPENAPI_GEN) $(SOURCES)
	CGO_ENABLED=0 go build -o build/$(BINARY) $(BUILD_FLAGS) -ldflags "$(LDFLAGS)" .

build/linux/$(BINARY): go.mod $(OPENAPI_GEN) $(SOURCES)
	GOOS=linux GOARCH=amd64 CGO_ENABLED=0 go build $(BUILD_FLAGS) -o build/linux/$(BINARY) -ldflags "$(LDFLAGS)" .

build/osx/$(BINARY): go.mod $(OPENAPI_GEN) $(SOURCES)
	GOOS=darwin GOARCH=amd64 CGO_ENABLED=0 go build $(BUILD_FLAGS) -o build/osx/$(BINARY) -ldflags "$(LDFLAGS)" .

build.docker: build.linux
	docker build --rm -t "$(IMAGE):$(TAG)" -f $(DOCKERFILE) .

build.push: build.docker
	docker push "$(IMAGE):$(TAG)"
