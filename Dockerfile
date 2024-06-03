ARG BASE_IMAGE=registry.opensource.zalan.do/library/static:latest
FROM ${BASE_IMAGE}
LABEL maintainer="Team Teapot @ Zalando SE <team-teapot@zalando.de>"

ARG TARGETARCH

ADD build/linux/${TARGETARCH}/kube-metrics-adapter /

ENTRYPOINT ["/kube-metrics-adapter"]
