FROM registry.opensource.zalan.do/library/alpine-3.12:latest
MAINTAINER Team Teapot @ Zalando SE <team-teapot@zalando.de>

# add binary
ADD build/linux/kube-metrics-adapter /

ENTRYPOINT ["/kube-metrics-adapter"]
