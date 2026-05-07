FROM gcr.io/distroless/static-debian12:latest
LABEL maintainer="Team Teapot @ Zalando SE <team-teapot@zalando.de>"

ARG TARGETARCH

# add binary
ADD build/linux/${TARGETARCH}/custom-metrics-consumer /

ENTRYPOINT ["/custom-metrics-consumer"]
