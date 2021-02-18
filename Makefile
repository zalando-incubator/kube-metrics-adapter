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


default: build.local

clean:
	rm -rf build
	rm -rf $(OPENAPI)

test:
	go test -v -coverprofile=profile.cov $(GOPKGS)

check:
	go mod download
	golangci-lint run --timeout=2m ./...


$(OPENAPI): go.mod
	go run k8s.io/kube-openapi/cmd/openapi-gen \
		--go-header-file hack/boilerplate.go.txt \
		--logtostderr \
		-i k8s.io/metrics/pkg/apis/custom_metrics,k8s.io/metrics/pkg/apis/custom_metrics/v1beta1,k8s.io/metrics/pkg/apis/custom_metrics/v1beta2,k8s.io/metrics/pkg/apis/external_metrics,k8s.io/metrics/pkg/apis/external_metrics/v1beta1,k8s.io/metrics/pkg/apis/metrics,k8s.io/metrics/pkg/apis/metrics/v1beta1,k8s.io/apimachinery/pkg/apis/meta/v1,k8s.io/apimachinery/pkg/api/resource,k8s.io/apimachinery/pkg/version,k8s.io/api/core/v1 \
		-p pkg/api/generated/openapi \
		-o . \
		-O zz_generated.openapi \
		-r /dev/null

build.local: build/$(BINARY)
build.linux: build/linux/$(BINARY)
build.osx: build/osx/$(BINARY)

build/$(BINARY): go.mod $(SOURCES) $(OPENAPI)
	CGO_ENABLED=0 go build -o build/$(BINARY) $(BUILD_FLAGS) -ldflags "$(LDFLAGS)" .

build/linux/$(BINARY): go.mod $(SOURCES) $(OPENAPI)
	GOOS=linux GOARCH=amd64 CGO_ENABLED=0 go build $(BUILD_FLAGS) -o build/linux/$(BINARY) -ldflags "$(LDFLAGS)" .

build/osx/$(BINARY): go.mod $(SOURCES) $(OPENAPI)
	GOOS=darwin GOARCH=amd64 CGO_ENABLED=0 go build $(BUILD_FLAGS) -o build/osx/$(BINARY) -ldflags "$(LDFLAGS)" .

build.docker: build.linux
	docker build --rm -t "$(IMAGE):$(TAG)" -f $(DOCKERFILE) .

build.push: build.docker
	docker push "$(IMAGE):$(TAG)"
