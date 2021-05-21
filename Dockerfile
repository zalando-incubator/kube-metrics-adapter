FROM registry.opensource.zalan.do/library/alpine-3.12:latest
LABEL maintainer="Team Teapot @ Zalando SE <team-teapot@zalando.de>"

RUN apk add --no-cache tzdata

# add binary
ADD build/linux/kube-metrics-adapter /

ENTRYPOINT ["/kube-metrics-adapter"]
