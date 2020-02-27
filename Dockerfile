ARG GO_VERSION=1.13

FROM golang:${GO_VERSION}-alpine AS builder

RUN apk add --update --no-cache bash make curl git mercurial bzr

ENV GOFLAGS="-mod=readonly"

RUN mkdir -p /build
WORKDIR /build

COPY go.* /build/
RUN go mod download

COPY . /build
RUN make build.linux

FROM alpine:3.9

COPY --from=builder /build/build/linux/kube-metrics-adapter /

ENTRYPOINT ["/kube-metrics-adapter"]
