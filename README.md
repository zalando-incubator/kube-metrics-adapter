# kube-metrics-adapter
[![Build Status](https://travis-ci.org/zalando-incubator/kube-metrics-adapter.svg?branch=master)](https://travis-ci.org/zalando-incubator/kube-metrics-adapter)
[![Coverage Status](https://coveralls.io/repos/github/zalando-incubator/kube-metrics-adapter/badge.svg?branch=master)](https://coveralls.io/github/zalando-incubator/kube-metrics-adapter?branch=master)

Kube Metrics Adapter is a general purpose metrics adapter for Kubernetes that
can collect and serve custom and external metrics for Horizontal Pod
Autoscaling.

It supports scaling based on [Prometheus metrics](https://prometheus.io/), [SQS queues](https://aws.amazon.com/sqs/) and others out of the box.

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

## Kubernetes compatibility

Like the [support
policy](https://kubernetes.io/docs/setup/release/version-skew-policy/) offered
for Kubernetes, this project aims to support the latest three minor releases of
Kubernetes.

Currently the default supported API is `autoscaling/v2beta1`. However we aim to
move to `autoscaling/v2beta2` (available since `v1.12`) in the near future as
this adds a lot of improvements over `v2beta1`. The move to `v2beta2` will most
likely happen as soon as [GKE adds support for it](https://issuetracker.google.com/issues/135624588).

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

| Metric | Description | Type | K8s Versions |
| ------------ | -------------- | ------- | -- |
| *custom* | No predefined metrics. Metrics are generated from user defined queries. | Pods | `>=1.10` |

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
    metric-config.pods.requests-per-second.json-path/scheme: "https"
    metric-config.pods.requests-per-second.json-path/aggregator: "max"
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

The other configuration options `path`, `port` and `scheme` specify where the metrics
endpoint is exposed on the pod. The `path` and `port` options do not have default values
so they must be defined. The `scheme` is optional and defaults to `http`.

The `aggregator` configuration option specifies the aggregation function used to aggregate
values of JSONPath expressions that evaluate to arrays/slices of numbers.
It's optional but when the expression evaluates to an array/slice, it's absence will
produce an error. The supported aggregation functions are `avg`, `max`, `min` and `sum`.

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

| Metric | Description | Type | Kind | K8s Versions |
| ------------ | -------------- | ------- | -- | -- |
| `prometheus-query` | Generic metric which requires a user defined query. | External | | `>=1.10` |
| *custom* | No predefined metrics. Metrics are generated from user defined queries. | Object | *any* | `>=1.10` |

### Example: External Metric

This is an example of an HPA configured to get metrics based on a Prometheus
query. The query is defined in the annotation
`metric-config.external.prometheus-query.prometheus/processed-events-per-second`
where `processed-events-per-second` is the query name which will be associated
with the result of the query. A matching `query-name` label must be defined in
the `matchLabels` of the metric definition. This allows having multiple
prometheus queries associated with a single HPA.

```yaml
apiVersion: autoscaling/v2beta1
kind: HorizontalPodAutoscaler
metadata:
  name: myapp-hpa
  annotations:
    # This annotation is optional.
    # If specified, then this prometheus server is used,
    # instead of the prometheus server specified as the CLI argument `--prometheus-server`.
    metric-config.external.prometheus-query.prometheus/prometheus-server: http://prometheus.my-namespace.svc
    # metric-config.<metricType>.<metricName>.<collectorName>/<configKey>
    # <configKey> == query-name
    metric-config.external.prometheus-query.prometheus/processed-events-per-second: |
      scalar(sum(rate(event-service_events_count{application="event-service",processed="true"}[1m])))
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
      metricName: prometheus-query
      metricSelector:
        matchLabels:
          query-name: processed-events-per-second
      targetAverageValue: 10
```

### Example: Object Metric [DEPRECATED]

> _Note: Prometheus Object metrics are **deprecated** and will most likely be
> removed in the future. Use the Prometheus External metrics instead as described
> above._

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
        kind: Pod
        name: dummy-pod
      targetValue: 10 # this will be treated as targetAverageValue
```

_Note:_ The HPA object requires an `Object` to be specified. However when a Prometheus metric is used there is no need
for this object. But to satisfy the schema we specify a dummy pod called `dummy-pod`.


## Skipper collector

The skipper collector is a simple wrapper around the Prometheus collector to
make it easy to define an HPA for scaling based on ingress metrics when
[skipper](https://github.com/zalando/skipper) is used as the ingress
implementation in your cluster. It assumes you are collecting Prometheus
metrics from skipper and it provides the correct Prometheus queries out of the
box so users don't have to define those manually.

### Supported metrics

| Metric | Description | Type | Kind | K8s Versions |
| ----------- | -------------- | ------ | ---- | ---- |
| `requests-per-second` | Scale based on requests per second for a certain ingress. | Object | `Ingress` | `>=1.14` (can work with `>=1.10`) |

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
      averageValue: 10 # Only works with Kubernetes >=1.14
      # for Kubernetes <1.14 you can use `targetValue` instead:
      targetValue: 10 # this must be set, but has no effect if `averageValue` is defined.
                      # Otherwise it will be treated as targetAverageValue
```

### Metric weighting based on backend

Skipper supports sending traffic to different backend based on annotations present on the
`Ingress` object. When the metric name is specified without a backend as `requests-per-second`
then the number of replicas will be calculated based on the full traffic served by that ingress.
If however only the traffic being routed to a specific backend should be used then the
backend name can be specified as a metric name like `requests-per-second,backend1` which would
return the requests-per-second being sent to the `backend1`. The ingress annotation where
the backend weights can be obtained can be specified through the flag `--skipper-backends-annotation`.


**Note:** For Kubernetes `<v1.14` the HPA does not support `averageValue` for
metrics of type `Object`. In case of requests per second it does not make sense
to scale on a summed value because you can not make the total requests per
second go down by adding more pods. For this reason the skipper collector will
automatically treat the value you define in `targetValue` as an average per pod
instead of a total sum.

**ONLY use `targetValue` if you are on Kubernetes
`<1.14`, it is not as percise as using `averageValue` and will not be supported
after Kubernetes `v1.16` is released according to the [support policy](https://kubernetes.io/docs/setup/release/version-skew-policy/).**

## AWS collector

The AWS collector allows scaling based on external metrics exposed by AWS
services e.g. SQS queue lengths.

### AWS IAM role

To integrate with AWS, the controller needs to run on nodes with
access to AWS API. Additionally the controller have to have a role
with the following policy to get all required data from AWS:

```yaml
PolicyDocument:
  Statement:
    - Action: 'sqs:GetQueueUrl'
      Effect: Allow
      Resource: '*'
    - Action: 'sqs:GetQueueAttributes'
      Effect: Allow
      Resource: '*'
    - Action: 'sqs:ListQueues'
      Effect: Allow
      Resource: '*'
    - Action: 'sqs:ListQueueTags'
      Effect: Allow
      Resource: '*'
  Version: 2012-10-17
```

### Supported metrics

| Metric | Description | Type | K8s Versions |
| ------------ | ------- | -- | -- |
| `sqs-queue-length` | Scale based on SQS queue length | External | `>=1.10` |

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

| Metric | Description | Type | K8s Versions |
| ------------ | ------- | -- | -- |
| `zmon-check` | Scale based on any ZMON check results | External | `>=1.10` |

### Example

This is an example of an HPA that will scale based on the specified value
exposed by a ZMON check with id `1234`.

```yaml
apiVersion: autoscaling/v2beta1
kind: HorizontalPodAutoscaler
metadata:
  name: myapp-hpa
  annotations:
    # metric-config.<metricType>.<metricName>.<collectorName>/<configKey>
    metric-config.external.zmon-check.zmon/key: "custom.*"
    metric-config.external.zmon-check.zmon/tag-application: "my-custom-app-*"
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
          tag-application: my-custom-app
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

The `tag-<name>` labels defines the tags used for the kariosDB query. In a
normal ZMON setup the following tags will be available:

* `application`
* `alias` (name of Kubernetes cluster)
* `entity` - full ZMON entity ID.

`aggregators` defines the aggregation functions applied to the metrics query.
For instance if you define the entity filter
`type=kube_pod,application=my-custom-app` you might get three entities back and
then you might want to get an average over the metrics for those three
entities. This would be possible by using the `avg` aggregator. The default
aggregator is `last` which returns only the latest metric point from the
query. The supported aggregation functions are `avg`, `dev`, `count`,
`first`, `last`, `max`, `min`, `sum`, `diff`. See the [KariosDB docs](https://kairosdb.github.io/docs/build/html/restapi/Aggregators.html) for
details.

The `duration` defines the duration used for the timeseries query. E.g. if you
specify a duration of `5m` then the query will return metric points for the
last 5 minutes and apply the specified aggregation with the same duration .e.g
`max(5m)`.

The annotations `metric-config.external.zmon-check.zmon/key` and
`metric-config.external.zmon-check.zmon/tag-<name>` can be optionally used if
you need to define a `key` or other `tag` with a "star" query syntax like
`values.*`. This *hack* is in place because it's not allowed to use `*` in the
metric label definitions. If both annotations and corresponding label is
defined, then the annotation takes precedence.
