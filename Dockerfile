ARG BASE_IMAGE=registry.opensource.zalan.do/library/alpine-3.13:latest
FROM ${BASE_IMAGE}
LABEL maintainer="Team Teapot @ Zalando SE <team-teapot@zalando.de>"

RUN apk add --no-cache tzdata

ARG TARGETARCH

ADD build/linux/${TARGETARCH}/kube-metrics-adapter /

ENTRYPOINT ["/kube-metrics-adapter"]
