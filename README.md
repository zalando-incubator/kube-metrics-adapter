# kube-metrics-adapter
[![Build Status](https://travis-ci.org/zalando-incubator/kube-metrics-adapter.svg?branch=master)](https://travis-ci.org/zalando-incubator/kube-metrics-adapter)

Kube Metrics Adapter is a general purpose metrics adapter for Kubernetes that
can collect and serve custom and external metrics for Horizontal Pod
Autoscaling.

It discovers Horizontal Pod Autoscaling resources and starts to collect the
requested metrics and stores them in memory. It's implemented using the
[custom-metrics-apiserver](https://github.com/kubernetes-incubator/custom-metrics-apiserver)
library.

Here's an example of a `HorizontalPodAutoscaler` resource configured to get
`requests-per-second` metrics from each pod of the deployment `myapp`.

```yaml
apiVersion: autoscaling/v2beta1
kind: HorizontalPodAutoscaler
metadata:
  name: myapp-hpa
  annotations:
    # metric-config.<metricType>.<metricName>.<collectorName>/<configKey>
    metric-config.pods.requests-per-second.json-path/json-key: "$.http_server.rps"
    metric-config.pods.requests-per-second.json-path/path: /metrics
    metric-config.pods.requests-per-second.json-path/port: "9090"
spec:
  scaleTargetRef:
    apiVersion: apps/v1
    kind: Deployment
    name: myapp
  minReplicas: 1
  maxReplicas: 10
  metrics:
  - type: Pods
    pods:
      metricName: requests-per-second
      targetAverageValue: 1k
```

The `metric-config.*` annotations are used by the `kube-metrics-adapter` to
configure a collector for getting the metrics. In the above example it
configures a *json-path pod collector*.

## Building

This project uses [Go modules](https://github.com/golang/go/wiki/Modules) as
introduced in Go 1.11 therefore you need Go >=1.11 installed in order to build.
If using Go 1.11 you also need to [activate Module
support](https://github.com/golang/go/wiki/Modules#installing-and-activating-module-support).

Assuming Go has been setup with module support it can be built simply by running:

```sh
export GO111MODULE=on # needed if the project is checked out in your $GOPATH.
$ make
```

## Collectors

Collectors are different implementations for getting metrics requested by an
HPA resource. They are configured based on HPA resources and started on-demand by the
`kube-metrics-adapter` to only collect the metrics required for scaling the application.

The collectors are configured either simply based on the metrics defined in an
HPA resource, or via additional annotations on the HPA resource.

## Pod collector

The pod collector allows collecting metrics from each pod matched by the HPA.
Currently only `json-path` collection is supported.

### Supported metrics

| Metric | Description | Type |
| ------------ | -------------- | ------- |
| *custom* | No predefined metrics. Metrics are generated from user defined queries. | Pods |

### Example

This is an example of using the pod collector to collect metrics from a json
metrics endpoint of each pod matched by the HPA.

```yaml
apiVersion: autoscaling/v2beta1
kind: HorizontalPodAutoscaler
metadata:
  name: myapp-hpa
  annotations:
    # metric-config.<metricType>.<metricName>.<collectorName>/<configKey>
    metric-config.pods.requests-per-second.json-path/json-key: "$.http_server.rps"
    metric-config.pods.requests-per-second.json-path/path: /metrics
    metric-config.pods.requests-per-second.json-path/port: "9090"
spec:
  scaleTargetRef:
    apiVersion: apps/v1
    kind: Deployment
    name: myapp
  minReplicas: 1
  maxReplicas: 10
  metrics:
  - type: Pods
    pods:
      metricName: requests-per-second
      targetAverageValue: 1k
```

The pod collector is configured through the annotations which specify the
collector name `json-path` and a set of configuration options for the
collector. `json-key` defines the json-path query for extracting the right
metric. This assumes the pod is exposing metrics in JSON format. For the above
example the following JSON data would be expected:

```json
{
  "http_server": {
    "rps": 0.5
  }
}
```

The json-path query support depends on the
[github.com/oliveagle/jsonpath](https://github.com/oliveagle/jsonpath) library.
See the README for possible queries. It's expected that the metric you query
returns something that can be turned into a `float64`.

The other configuration options `path` and `port` specifies where the metrics
endpoint is exposed on the pod. There's no default values, so they must be
defined.

## Prometheus collector

The Prometheus collector is a generic collector which can map Prometheus
queries to metrics that can be used for scaling. This approach is different
from how it's done in the
[k8s-prometheus-adapter](https://github.com/DirectXMan12/k8s-prometheus-adapter)
where all available Prometheus metrics are collected
and transformed into metrics which the HPA can scale on, and there is no
possibility to do custom queries.
With the approach implemented here, users can define custom queries and only metrics
returned from those queries will be available, reducing the total number of
metrics stored.

One downside of this approach is that bad performing queries can slow down/kill
Prometheus, so it can be dangerous to allow in a multi tenant cluster. It's
also not possible to restrict the available metrics using something like RBAC
since any user would be able to create the metrics based on a custom query.

I still believe custom queries are more useful, but it's good to be aware of
the trade-offs between the two approaches.

### Supported metrics

| Metric | Description | Type | Kind |
| ------------ | -------------- | ------- | -- |
| *custom* | No predefined metrics. Metrics are generated from user defined queries. | Object | *any* |

### Example

This is an example of an HPA configured to get metrics based on a Prometheus
query. The query is defined in the annotation
`metric-config.object.processed-events-per-second.prometheus/query` where
`processed-events-per-second` is the metric name which will be associated with
the result of the query.

It also specifies an annotation
`metric-config.object.processed-events-per-second.prometheus/per-replica` which
instructs the collector to treat the results as an average over all pods
targeted by the HPA. This makes it possible to mimic the behavior of
`targetAverageValue` which is not implemented for metric type `Object` as of
Kubernetes v1.10. ([It will most likely come in v1.12](https://github.com/kubernetes/kubernetes/pull/64097#event-1696222479)).

```yaml
apiVersion: autoscaling/v2beta1
kind: HorizontalPodAutoscaler
metadata:
  name: myapp-hpa
  annotations:
    # metric-config.<metricType>.<metricName>.<collectorName>/<configKey>
    metric-config.object.processed-events-per-second.prometheus/query: |
      scalar(sum(rate(event-service_events_count{application="event-service",processed="true"}[1m])))
    metric-config.object.processed-events-per-second.prometheus/per-replica: "true"
spec:
  scaleTargetRef:
    apiVersion: apps/v1
    kind: Deployment
    name: custom-metrics-consumer
  minReplicas: 1
  maxReplicas: 10
  metrics:
  - type: Object
    object:
      metricName: processed-events-per-second
      target:
        apiVersion: v1
        kind: Service
        name: event-service
      targetValue: 10 # this will be treated as targetAverageValue
```

## Skipper collector

The skipper collector is a simple wrapper around the Prometheus collector to
make it easy to define an HPA for scaling based on ingress metrics when
[skipper](https://github.com/zalando/skipper) is used as the ingress
implementation in your cluster. It assumes you are collecting Prometheus
metrics from skipper and it provides the correct Prometheus queries out of the
box so users don't have to define those manually.

### Supported metrics

| Metric | Description | Type | Kind |
| ----------- | -------------- | ------ | ---- |
| `requests-per-second` | Scale based on requests per second for a certain ingress. | Object | `Ingress` |

### Example

This is an example of an HPA that will scale based on `requests-per-second` for
an ingress called `myapp`.

```yaml
apiVersion: autoscaling/v2beta1
kind: HorizontalPodAutoscaler
metadata:
  name: myapp-hpa
spec:
  scaleTargetRef:
    apiVersion: apps/v1
    kind: Deployment
    name: myapp
  minReplicas: 1
  maxReplicas: 10
  metrics:
  - type: Object
    object:
      metricName: requests-per-second
      target:
        apiVersion: extensions/v1beta1
        kind: Ingress
        name: myapp
      targetValue: 10 # this will be treated as targetAverageValue
```

**Note:** As of Kubernetes v1.10 the HPA does not support `targetAverageValue` for
metrics of type `Object`. In case of requests per second it does not make sense
to scale on a summed value because you can not make the total requests per
second go down by adding more pods. For this reason the skipper collector will
automatically treat the value you define in `targetValue` as an average per pod
instead of a total sum.

## AWS collector

The AWS collector allows scaling based on external metrics exposed by AWS
services e.g. SQS queue lengths.

### Supported metrics

| Metric | Description | Type |
| ------------ | ------- | -- |
| `sqs-queue-length` | Scale based on SQS queue length | External |

### Example

This is an example of an HPA that will scale based on the length of an SQS
queue.

```yaml
apiVersion: autoscaling/v2beta1
kind: HorizontalPodAutoscaler
metadata:
  name: myapp-hpa
spec:
  scaleTargetRef:
    apiVersion: apps/v1
    kind: Deployment
    name: custom-metrics-consumer
  minReplicas: 1
  maxReplicas: 10
  metrics:
  - type: External
    external:
      metricName: sqs-queue-length
      metricSelector:
        matchLabels:
          queue-name: foobar
          region: eu-central-1
      targetAverageValue: 30
```

The `matchLabels` are used by `kube-metrics-adapter` to configure a collector
that will get the queue length for an SQS queue named `foobar` in region
`eu-central-1`.

The AWS account of the queue currently depends on how `kube-metrics-adapter` is
configured to get AWS credentials. The normal assumption is that you run the
adapter in a cluster running in the AWS account where the queue is defined.
Please open an issue if you would like support for other use cases.

## ZMON collector

The ZMON collector allows scaling based on external metrics exposed by
[ZMON](https://github.com/zalando/zmon) checks.

### Supported metrics

| Metric | Description | Type |
| ------------ | ------- | -- |
| `zmon-check` | Scale based on any ZMON check results | External |

### Example

This is an example of an HPA that will scale based on the specifed value
exposed by a ZMON check with id `1234`.

```yaml
apiVersion: autoscaling/v2beta1
kind: HorizontalPodAutoscaler
metadata:
  name: myapp-hpa
spec:
  scaleTargetRef:
    apiVersion: apps/v1
    kind: Deployment
    name: custom-metrics-consumer
  minReplicas: 1
  maxReplicas: 10
  metrics:
  - type: External
    external:
      metricName: zmon-check
      metricSelector:
        matchLabels:
          check-id: "1234" # the ZMON check to query for metrics
          key: "custom.value"
          entity-type: kube_pod
          entity-application: my-custom-app
          aggregators: avg # comma separated list of aggregation functions, default: last
          duration: 5m # default: 10m
      targetAverageValue: 30
```

The `check-id` specifies the ZMON check to query for the metrics. `key`
specifies the JSON key in the check output to extract the metric value from.
E.g. if you have a check which returns the following data:

```json
{
    "custom": {
        "value": 1.0
    },
    "other": {
        "value": 3.0
    }
}
```

Then the value `1.0` would be returned when the key is defined as `custom.value`.

The `entity-<name>` labels defines the entity filter used for looking up check
entities in ZMON before the query. e.g. the above example would result in the
entity filter: `type=kube_pod,application=my-custom-app` which would limit the
resulting query to entities matching that entity filter.

`aggregators` defines the aggregation functions applied to the metrics query.
For instance if you define the entity filter
`type=kube_pod,application=my-custom-app` you might get three entities back and
then you might want to get an average over the metrics for those three
entities. This would be possible by using the `avg` aggrator. The default
aggregator is `last` which returns only the lastest metric point from the
query. The supported aggregation functions are `avg`, `dev`, `count`,
`first`, `last`, `max`, `min`, `sum`, `diff`. See the [KariosDB docs](https://kairosdb.github.io/docs/build/html/restapi/Aggregators.html) for
details.

The `duration` defines the duration used for the timeseries query. E.g. if you
specify a duration of `5m` then the query will return metric points for the
last 5 minutes and apply the specified aggration with the same duration .e.g
`max(5m)`.
