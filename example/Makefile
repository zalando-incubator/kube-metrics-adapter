.PHONY: clean test check build.local build.linux build.osx build.docker build.push

BINARY        ?= custom-metrics-consumer
VERSION       ?= $(shell git describe --tags --always --dirty)
IMAGE         ?= mikkeloscar/$(BINARY)
TAG           ?= $(VERSION)
SOURCES       = $(shell find . -name '*.go')
DOCKERFILE    ?= Dockerfile
GOPKGS        = $(shell go list ./...)
BUILD_FLAGS   ?= -v
LDFLAGS       ?= -X main.version=$(VERSION) -w -s

default: build.local

clean:
	rm -rf build

test:
	go test -v $(GOPKGS)

check:
	golint $(GOPKGS)
	go vet -v $(GOPKGS)

build.local: build/$(BINARY)
build.linux: build/linux/$(BINARY)
build.osx: build/osx/$(BINARY)

build/$(BINARY): $(SOURCES)
	CGO_ENABLED=0 go build -o build/$(BINARY) $(BUILD_FLAGS) -ldflags "$(LDFLAGS)" .

build/linux/$(BINARY): $(SOURCES)
	GOOS=linux GOARCH=amd64 CGO_ENABLED=0 go build $(BUILD_FLAGS) -o build/linux/$(BINARY) -ldflags "$(LDFLAGS)" .

build/linux/amd64/$(BINARY): $(SOURCES)
	GOOS=linux GOARCH=amd64 CGO_ENABLED=0 go build $(BUILD_FLAGS) -o build/linux/amd64/$(BINARY) -ldflags "$(LDFLAGS)" .

build/linux/arm64/$(BINARY): $(SOURCES)
	GOOS=linux GOARCH=arm64 CGO_ENABLED=0 go build $(BUILD_FLAGS) -o build/linux/arm64/$(BINARY) -ldflags "$(LDFLAGS)" .

build/osx/$(BINARY): $(SOURCES)
	GOOS=darwin GOARCH=amd64 CGO_ENABLED=0 go build $(BUILD_FLAGS) -o build/osx/$(BINARY) -ldflags "$(LDFLAGS)" .

build.push: build/linux/amd64/$(BINARY) build/linux/arm64/$(BINARY)
	docker buildx build --rm --build-arg -t "$(IMAGE):$(TAG)" --platform linux/amd64,linux/arm64 --push .
