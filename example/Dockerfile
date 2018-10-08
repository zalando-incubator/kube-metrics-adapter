FROM registry.opensource.zalan.do/stups/alpine:latest
MAINTAINER Team Teapot @ Zalando SE <team-teapot@zalando.de>

# add binary
ADD build/linux/custom-metrics-consumer /

ENTRYPOINT ["/custom-metrics-consumer"]
