.PHONY: clean test check build.local build.linux build.osx build.docker build.push

BINARY        ?= kube-metrics-adapter
VERSION       ?= $(shell git describe --tags --always --dirty)
IMAGE         ?= registry-write.opensource.zalan.do/teapot/$(BINARY)
TAG           ?= $(VERSION)
SOURCES       = $(shell find . -name '*.go')
DOCKERFILE    ?= Dockerfile
GOPKGS        = $(shell go list ./...)
BUILD_FLAGS   ?= -v
OPENAPI       ?= pkg/api/generated/openapi/zz_generated.openapi.go
LDFLAGS       ?= -X main.version=$(VERSION) -w -s
CRD_SOURCES    = $(shell find pkg/apis/zalando.org -name '*.go')
CRD_TYPE_SOURCE = pkg/apis/zalando.org/v1/types.go
GENERATED_CRDS = docs/scaling_schedules_crd.yaml
GENERATED      = pkg/apis/zalando.org/v1/zz_generated.deepcopy.go


default: build.local

clean:
	rm -rf build
	rm -rf $(OPENAPI)

test: $(GENERATED)
	go test -v -coverprofile=profile.cov $(GOPKGS)

check: $(GENERATED)
	go mod download
	golangci-lint run --timeout=2m ./...


$(GENERATED): go.mod $(CRD_TYPE_SOURCE) $(OPENAPI)
	./hack/update-codegen.sh

$(GENERATED_CRDS): $(GENERATED) $(CRD_SOURCES)
	go run sigs.k8s.io/controller-tools/cmd/controller-gen crd:crdVersions=v1 paths=./pkg/apis/... output:crd:dir=docs
	mv docs/zalando.org_clusterscalingschedules.yaml docs/cluster_scaling_schedules_crd.yaml
	mv docs/zalando.org_scalingschedules.yaml docs/scaling_schedules_crd.yaml

$(OPENAPI): go.mod
	go run k8s.io/kube-openapi/cmd/openapi-gen \
		--go-header-file hack/boilerplate.go.txt \
		--logtostderr \
		-i k8s.io/metrics/pkg/apis/custom_metrics,k8s.io/metrics/pkg/apis/custom_metrics/v1beta1,k8s.io/metrics/pkg/apis/custom_metrics/v1beta2,k8s.io/metrics/pkg/apis/external_metrics,k8s.io/metrics/pkg/apis/external_metrics/v1beta1,k8s.io/metrics/pkg/apis/metrics,k8s.io/metrics/pkg/apis/metrics/v1beta1,k8s.io/apimachinery/pkg/apis/meta/v1,k8s.io/apimachinery/pkg/api/resource,k8s.io/apimachinery/pkg/version,k8s.io/api/core/v1 \
		-p pkg/api/generated/openapi \
		-o . \
		-O zz_generated.openapi \
		-r /dev/null

build.local: build/$(BINARY) $(GENERATED_CRDS)
build.linux: build/linux/$(BINARY)
build.linux.amd64: build/linux/amd64/$(BINARY)
build.linux.arm64: build/linux/arm64/$(BINARY)


build/$(BINARY): go.mod $(SOURCES) $(GENERATED)
	CGO_ENABLED=0 go build -o build/$(BINARY) $(BUILD_FLAGS) -ldflags "$(LDFLAGS)" .

build/linux/$(BINARY): go.mod $(SOURCES) $(GENERATED)
	GOOS=linux GOARCH=amd64 CGO_ENABLED=0 go build $(BUILD_FLAGS) -o build/linux/$(BINARY) -ldflags "$(LDFLAGS)" .

build/linux/amd64/$(BINARY): go.mod $(SOURCES)
	GOOS=linux GOARCH=amd64 CGO_ENABLED=0 go build $(BUILD_FLAGS) -o build/linux/amd64/$(BINARY) -ldflags "$(LDFLAGS)" .

build/linux/arm64/$(BINARY): go.mod $(SOURCES)
	GOOS=linux GOARCH=arm64 CGO_ENABLED=0 go build $(BUILD_FLAGS) -o build/linux/arm64/$(BINARY) -ldflags "$(LDFLAGS)" .

build.docker: build.linux
	docker build --rm -t "$(IMAGE):$(TAG)" -f $(DOCKERFILE) --build-arg TARGETARCH= .

build.push: build.docker
	docker push "$(IMAGE):$(TAG)"
