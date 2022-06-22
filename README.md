# kube-metrics-adapter
[![Build Status](https://travis-ci.org/zalando-incubator/kube-metrics-adapter.svg?branch=master)](https://travis-ci.org/zalando-incubator/kube-metrics-adapter)
[![Coverage Status](https://coveralls.io/repos/github/zalando-incubator/kube-metrics-adapter/badge.svg?branch=master)](https://coveralls.io/github/zalando-incubator/kube-metrics-adapter?branch=master)

Kube Metrics Adapter is a general purpose metrics adapter for Kubernetes that
can collect and serve custom and external metrics for Horizontal Pod
Autoscaling.

It supports scaling based on [Prometheus metrics](https://prometheus.io/), [SQS queues](https://aws.amazon.com/sqs/) and others out of the box.

It discovers Horizontal Pod Autoscaling resources and starts to collect the
requested metrics and stores them in memory. It's implemented using the
[custom-metrics-apiserver](https://github.com/kubernetes-sigs/custom-metrics-apiserver)
library.

Here's an example of a `HorizontalPodAutoscaler` resource configured to get
`requests-per-second` metrics from each pod of the deployment `myapp`.

```yaml
apiVersion: autoscaling/v2beta2
kind: HorizontalPodAutoscaler
metadata:
  name: myapp-hpa
  annotations:
    # metric-config.<metricType>.<metricName>.<collectorType>/<configKey>
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
      metric:
        name: requests-per-second
      target:
        averageValue: 1k
        type: AverageValue
```

The `metric-config.*` annotations are used by the `kube-metrics-adapter` to
configure a collector for getting the metrics. In the above example it
configures a *json-path pod collector*.

## Kubernetes compatibility

Like the [support
policy](https://kubernetes.io/docs/setup/release/version-skew-policy/) offered
for Kubernetes, this project aims to support the latest three minor releases of
Kubernetes.

The default supported API is `autoscaling/v2beta2` (available since `v1.12`).
This API MUST be available in the cluster which is the default. However for
GKE, this requires GKE v1.15.7 according to this [GKE
Issue](https://issuetracker.google.com/issues/135624588).

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
| *custom* | No predefined metrics. Metrics are generated from user defined queries. | Pods | `>=1.12` |

### Example

This is an example of using the pod collector to collect metrics from a json
metrics endpoint of each pod matched by the HPA.

```yaml
apiVersion: autoscaling/v2beta2
kind: HorizontalPodAutoscaler
metadata:
  name: myapp-hpa
  annotations:
    # metric-config.<metricType>.<metricName>.<collectorType>/<configKey>
    metric-config.pods.requests-per-second.json-path/json-key: "$.http_server.rps"
    metric-config.pods.requests-per-second.json-path/path: /metrics
    metric-config.pods.requests-per-second.json-path/port: "9090"
    metric-config.pods.requests-per-second.json-path/scheme: "https"
    metric-config.pods.requests-per-second.json-path/aggregator: "max"
    metric-config.pods.requests-per-second.json-path/interval: "60s" # optional
    metric-config.pods.requests-per-second.json-path/min-pod-ready-age: "30s" # optional
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
      metric:
        name: requests-per-second
      target:
        averageValue: 1k
        type: AverageValue
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
[github.com/spyzhov/ajson](https://github.com/spyzhov/ajson) library.
See the README for possible queries. It's expected that the metric you query
returns something that can be turned into a `float64`.

The other configuration options `path`, `port` and `scheme` specify where the metrics
endpoint is exposed on the pod. The `path` and `port` options do not have default values
so they must be defined. The `scheme` is optional and defaults to `http`.

The `aggregator` configuration option specifies the aggregation function used to aggregate
values of JSONPath expressions that evaluate to arrays/slices of numbers.
It's optional but when the expression evaluates to an array/slice, it's absence will
produce an error. The supported aggregation functions are `avg`, `max`, `min` and `sum`.

The `raw-query` configuration option specifies the query params to send along to the endpoint:
```yaml
  metric-config.pods.requests-per-second.json-path/path: /metrics
  metric-config.pods.requests-per-second.json-path/port: "9090"
  metric-config.pods.requests-per-second.json-path/raw-query: "foo=bar&baz=bop"
```
will create a URL like this:
```
http://<podIP>:9090/metrics?foo=bar&baz=bop
```

There are also configuration options for custom (connect and request) timeouts when querying pods for metrics:
```yaml
metric-config.pods.requests-per-second.json-path/request-timeout: 2s
metric-config.pods.requests-per-second.json-path/connect-timeout: 500ms
```

The default for both of the above values is 15 seconds.

The `min-pod-ready-age` configuration option instructs the service to start collecting metrics from the pods only if they are "older" (time elapsed after pod reached "Ready" state) than the specified amount of time.
This is handy when pods need to warm up before HPAs will start tracking their metrics.

The default value is 0 seconds.

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
| `prometheus-query` | Generic metric which requires a user defined query. | External | | `>=1.12` |
| *custom* | No predefined metrics. Metrics are generated from user defined queries. | Object | *any* | `>=1.12` |

### Example: External Metric

This is an example of an HPA configured to get metrics based on a Prometheus
query. The query is defined in the annotation
`metric-config.external.processed-events-per-second.prometheus/query`
where `processed-events-per-second` is the query name which will be associated
with the result of the query.
This allows having multiple prometheus queries associated with a single HPA.

```yaml
apiVersion: autoscaling/v2beta2
kind: HorizontalPodAutoscaler
metadata:
  name: myapp-hpa
  annotations:
    # This annotation is optional.
    # If specified, then this prometheus server is used,
    # instead of the prometheus server specified as the CLI argument `--prometheus-server`.
    metric-config.external.processed-events-per-second.prometheus/prometheus-server: http://prometheus.my-namespace.svc
    # metric-config.<metricType>.<metricName>.<collectorType>/<configKey>
    metric-config.external.processed-events-per-second.prometheus/query: |
      scalar(sum(rate(event-service_events_count{application="event-service",processed="true"}[1m])))
    metric-config.external.processed-events-per-second.prometheus/interval: "60s" # optional
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
      metric:
        name: processed-events-per-second
        selector:
          matchLabels:
            type: prometheus
      target:
        type: AverageValue
        averageValue: "10"
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
    # metric-config.<metricType>.<metricName>.<collectorType>/<configKey>
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
make it easy to define an HPA for scaling based on [Ingress][ingress] or
[RouteGroup][routegroup] metrics when
[skipper](https://github.com/zalando/skipper) is used as the ingress
implementation in your cluster. It assumes you are collecting Prometheus
metrics from skipper and it provides the correct Prometheus queries out of the
box so users don't have to define those manually.

[ingress]: https://kubernetes.io/docs/concepts/services-networking/ingress/
[routegroup]: https://opensource.zalando.com/skipper/kubernetes/routegroups/

### Supported metrics

| Metric | Description | Type | Kind | K8s Versions |
| ----------- | -------------- | ------ | ---- | ---- |
| `requests-per-second` | Scale based on requests per second for a certain ingress or routegroup. | Object | `Ingress`, `RouteGroup` | `>=1.19` |

### Example


#### Ingress

This is an example of an HPA that will scale based on `requests-per-second` for
an ingress called `myapp`.

```yaml
apiVersion: autoscaling/v2beta2
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
      describedObject:
        apiVersion: networking.k8s.io/v1
        kind: Ingress
        name: myapp
      metric:
        name: requests-per-second
        selector:
          matchLabels:
            backend: backend1 # optional backend
      target:
        averageValue: "10"
        type: AverageValue
```

#### RouteGroup

This is an example of an HPA that will scale based on `requests-per-second` for
a routegroup called `myapp`.

```yaml
apiVersion: autoscaling/v2beta2
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
      describedObject:
        apiVersion: zalando.org/v1
        kind: RouteGroup
        name: myapp
      metric:
        name: requests-per-second
        selector:
          matchLabels:
            backend: backend1 # optional backend
      target:
        averageValue: "10"
        type: AverageValue
```

### Metric weighting based on backend

Skipper supports sending traffic to different backends based on annotations
present on the `Ingress` object, or weights on the RouteGroup backends. By
default the number of replicas will be calculated based on the full traffic
served by that ingress/routegroup.  If however only the traffic being routed to
a specific backend should be used then the backend name can be specified via
the `backend` label under `matchLabels` for the metric.  The ingress annotation
where the backend weights can be obtained can be specified through the flag
`--skipper-backends-annotation`.

## InfluxDB collector

The InfluxDB collector maps [Flux](https://github.com/influxdata/flux) queries to metrics that can be used for scaling.

Note that the collector targets an [InfluxDB v2](https://v2.docs.influxdata.com/v2.0/get-started/) instance, that's why
we only support Flux instead of InfluxQL.

### Supported metrics

| Metric | Description | Type | Kind | K8s Versions |
| ------------ | -------------- | ------- | -- | -- |
| `flux-query` | Generic metric which requires a user defined query. | External | | `>=1.10` |

### Example: External Metric

This is an example of an HPA configured to get metrics based on a Flux query.
The query is defined in the annotation
`metric-config.external.<metricName>.influxdb/query` where `<metricName>` is
the query name which will be associated with the result of the query.  This
allows having multiple flux queries associated with a single HPA.

```yaml
apiVersion: autoscaling/v2beta2
kind: HorizontalPodAutoscaler
metadata:
  name: myapp-hpa
  annotations:
    # These annotations are optional.
    # If specified, then they are used for setting up the InfluxDB client properly,
    # instead of using the ones specified via CLI. Respectively:
    #  - --influxdb-address
    #  - --influxdb-token
    #  - --influxdb-org
    metric-config.external.queue-depth.influxdb/address: "http://influxdbv2.my-namespace.svc"
    metric-config.external.queue-depth.influxdb/token: "secret-token"
    # This could be either the organization name or the ID.
    metric-config.external.queue-depth.influxdb/org: "deadbeef"
    # metric-config.<metricType>.<metricName>.<collectorType>/<configKey>
    # <configKey> == query-name
    metric-config.external.queue-depth.influxdb/query: |
        from(bucket: "apps")
          |> range(start: -30s)
          |> filter(fn: (r) => r._measurement == "queue_depth")
          |> group()
          |> max()
          // Rename "_value" to "metricvalue" for letting the metrics server properly unmarshal the result.
          |> rename(columns: {_value: "metricvalue"})
          |> keep(columns: ["metricvalue"])
    metric-config.external.queue-depth.influxdb/interval: "60s" # optional
spec:
  scaleTargetRef:
    apiVersion: apps/v1
    kind: Deployment
    name: queryd-v1
  minReplicas: 1
  maxReplicas: 4
  metrics:
  - type: External
    external:
      metric:
        name: queue-depth
        selector:
          matchLabels:
            type: influxdb
      target:
        type: Value
        value: "1"
```

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
| `sqs-queue-length` | Scale based on SQS queue length | External | `>=1.12` |

### Example

This is an example of an HPA that will scale based on the length of an SQS
queue.

```yaml
apiVersion: autoscaling/v2beta2
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
      metric:
        name: my-sqs
        selector:
          matchLabels:
            type: sqs-queue-length
            queue-name: foobar
            region: eu-central-1
      target:
        averageValue: "30"
        type: AverageValue
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
| `zmon-check` | Scale based on any ZMON check results | External | `>=1.12` |

### Example

This is an example of an HPA that will scale based on the specified value
exposed by a ZMON check with id `1234`.

```yaml
apiVersion: autoscaling/v2beta2
kind: HorizontalPodAutoscaler
metadata:
  name: myapp-hpa
  annotations:
    # metric-config.<metricType>.<metricName>.<collectorType>/<configKey>
    metric-config.external.my-zmon-check.zmon/key: "custom.*"
    metric-config.external.my-zmon-check.zmon/tag-application: "my-custom-app-*"
    metric-config.external.my-zmon-check.zmon/interval: "60s" # optional
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
      metric:
          name: my-zmon-check
          selector:
            matchLabels:
              type: zmon
              check-id: "1234" # the ZMON check to query for metrics
              key: "custom.value"
              tag-application: my-custom-app
              aggregators: avg # comma separated list of aggregation functions, default: last
              duration: 5m # default: 10m
      target:
        averageValue: "30"
        type: AverageValue
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
query. The supported aggregation functions are `avg`, `count`,
`last`, `max`, `min`, `sum`, `diff`. See the [KariosDB docs](https://kairosdb.github.io/docs/build/html/restapi/Aggregators.html) for
details.

The `duration` defines the duration used for the timeseries query. E.g. if you
specify a duration of `5m` then the query will return metric points for the
last 5 minutes and apply the specified aggregation with the same duration .e.g
`max(5m)`.

The annotations `metric-config.external.my-zmon-check.zmon/key` and
`metric-config.external.my-zmon-check.zmon/tag-<name>` can be optionally used if
you need to define a `key` or other `tag` with a "star" query syntax like
`values.*`. This *hack* is in place because it's not allowed to use `*` in the
metric label definitions. If both annotations and corresponding label is
defined, then the annotation takes precedence.

## HTTP Collector

The http collector allows collecting metrics from an external endpoint specified in the HPA.
Currently only `json-path` collection is supported.

### Supported metrics

| Metric | Description | Type | K8s Versions |
| ------------ | -------------- | ------- | -- |
| *custom* | No predefined metrics. Metrics are generated from user defined queries. | Pods | `>=1.12` |

### Example

This is an example of using the HTTP collector to collect metrics from a json
metrics endpoint specified in the annotations.

```yaml
apiVersion: autoscaling/v2beta2
kind: HorizontalPodAutoscaler
metadata:
  name: myapp-hpa
  annotations:
    # metric-config.<metricType>.<metricName>.<collectorType>/<configKey>
    metric-config.external.unique-metric-name.json-path/json-key: "$.some-metric.value"
    metric-config.external.unique-metric-name.json-path/endpoint: "http://metric-source.app-namespace:8080/metrics"
    metric-config.external.unique-metric-name.json-path/aggregator: "max"
    metric-config.external.unique-metric-name.json-path/interval: "60s" # optional
spec:
  scaleTargetRef:
    apiVersion: apps/v1
    kind: Deployment
    name: myapp
  minReplicas: 1
  maxReplicas: 10
  metrics:
  - type: External
    external:
      metric:
        name: unique-metric-name
        selector:
          matchLabels:
            type: json-path
      target:
        averageValue: 1
        type: AverageValue
```

The HTTP collector similar to the Pod Metrics collector. The following
configuration values are supported:

- `json-key` to specify the JSON path of the metric to be queried
- `endpoint` the fully formed path to query for the metric. In the above example a Kubernetes _Service_
    in the namespace `app-namespace` is called.
- `aggregator` is only required if the metric is an array of values and specifies how the values
    are aggregated. Currently this option can support the values: `sum`, `max`, `min`, `avg`.

### Scrape Interval

It's possible to configure the scrape interval for each of the metric types via
an annotation:

```yaml
metric-config.<metricType>.<metricName>.<collectorType>/interval: "30s"
```

The default is `60s` but can be reduced to let the adapter collect metrics more
often.

## ScalingSchedule Collectors

The `ScalingSchedule` and `ClusterScalingSchedule` collectors allow
collecting time-based metrics from the respective CRD objects specified
in the HPA.

These collectors are disabled by default, you have to start the server
with the `--scaling-schedule` flag to enable it. Remember to deploy the CRDs
`ScalingSchedule` and `ClusterScalingSchedule` and allow the service
account used by the server to read, watch and list them.

### Supported metrics

| Metric | Description | Type | K8s Versions |
| ---------- | -------------- | ------- | -- |
| ObjectName | The metric is calculated and stored for each `ScalingSchedule` and `ClusterScalingSchedule` referenced in the HPAs | `ScalingSchedule` and `ClusterScalingSchedule` | `>=1.16` |

### Ramp-up and ramp-down feature

To avoid abrupt scaling due to time based metrics,the `SchalingSchedule`
collector has a feature of ramp-up and ramp-down the metric over a
specific period of time. The duration of the scaling window can be
configured individually in the `[Cluster]ScalingSchedule` object, via
the option `scalingWindowDurationMinutes` or globally for all scheduled
events, and defaults to a globally configured value if not specified.
The default for the latter is set to 10 minutes, but can be changed
using the `--scaling-schedule-default-scaling-window` flag.

This spreads the scale events around, creating less load on the other
components, and helping the rest of the metrics (like the CPU ones) to
adjust as well.

The [HPA algorithm][algo-details] does not make changes if the metric
change is less than the specified by the
`horizontal-pod-autoscaler-tolerance` flag:

> We'll skip scaling if the ratio is sufficiently close to 1.0 (within a
> globally-configurable tolerance, from the
> `--horizontal-pod-autoscaler-tolerance` flag, which defaults to 0.1.

With that in mind, the ramp-up and ramp-down feature divides the scaling
over the specified period of time in buckets, trying to achieve changes
bigger than the configured tolerance. The number of buckets defaults to
10 and can be configured by the `--scaling-schedule-ramp-steps` flag.

**Important**: note that the ramp-up and ramp-down feature can lead to
deployments achieving less than the specified number of pods, due to the
HPA 10% change rule and the ceiling function applied to the desired
number of the pods (check the [algorithm details][algo-details]). It
varies with the configured metric for `ScalingSchedule` events, the
number of pods and the configured `horizontal-pod-autoscaler-tolerance`
flag of your kubernetes installation. [This gist][gist] contains the code to
simulate the situations a deployment with different number of pods, with
a metric of 10000 can face with 10 buckets (max of 90% of the metric
returned) and 5 buckets (max of 80% of the metric returned). The ramp-up
and ramp-down feature can be disabled by setting
`--scaling-schedule-default-scaling-window` to 0 and abrupt scalings can
be handled via [scaling policies][policies].

[algo-details]: https://kubernetes.io/docs/tasks/run-application/horizontal-pod-autoscale/#algorithm-details
[gist]: https://gist.github.com/jonathanbeber/37f1f918ab7ef6101c6ce56cc2cef3a2
[policies]: https://kubernetes.io/docs/tasks/run-application/horizontal-pod-autoscale/#scaling-policies

### Example

This is an example of using the ScalingSchedule collectors to collect
metrics from a deployed kind of the CRD. First, the schedule object:

```yaml
apiVersion: zalando.org/v1
kind: ClusterScalingSchedule
metadata:
  name: "scheduling-event"
spec:
  schedules:
  - type: OneTime
    date: "2021-10-02T08:08:08+02:00"
    durationMinutes: 30
    value: 100
  - type: Repeating
    durationMinutes: 10
    value: 120
    period:
      startTime: "15:45"
      timezone: "Europe/Berlin"
      days:
      - Mon
      - Wed
      - Fri
```

This resource defines a scheduling event named `scheduling-event` with
two schedules of the kind `ClusterScalingSchedule`.

`ClusterScalingSchedule` objects aren't namespaced, what means it can be
referenced by any HPA in any namespace in the cluster. `ScalingSchedule`
have the exact same fields and behavior, but can be referenced just by
HPAs in the same namespace. The schedules can have the type `Repeating`
or `OneTime`.

This example configuration will generate the following result: at
`2021-10-02T08:08:08+02:00` for 30 minutes a metric with the value of
100 will be returned. Every Monday, Wednesday and Friday, starting at 15
hours and 45 minutes (Berlin time), a metric with the value of 120 will
be returned for 10 minutes. It's not the case of this example, but if multiple
schedules collide in time, the biggest value is returned.

Check the CRDs definitions
([ScalingSchedule](./docs/scaling_schedules_crd.yaml),
[ClusterScalingSchedule](./docs/cluster_scaling_schedules_crd.yaml)) for
a better understanding of the possible fields and their behavior.

An HPA can reference the deployed `ClusterScalingSchedule` object as
this example:

```yaml
apiVersion: autoscaling/v2beta2
kind: HorizontalPodAutoscaler
metadata:
  name: "myapp-hpa"
spec:
  scaleTargetRef:
    apiVersion: apps/v1
    kind: Deployment
    name: myapp
  minReplicas: 1
  maxReplicas: 15
  metrics:
  - type: Object
    object:
      describedObject:
        apiVersion: zalando.org/v1
        kind: ClusterScalingSchedule
        name: "scheduling-event"
      metric:
        name: "scheduling-event"
      target:
        type: AverageValue
        averageValue: "10"
```

The name of the metric is equal to the name of the referenced object.
The `target.averageValue` in this example is set to 10. This value will
be used by the HPA controller to define the desired number of pods,
based on the metric obtained (check the [HPA algorithm
details](https://kubernetes.io/docs/tasks/run-application/horizontal-pod-autoscale/#algorithm-details)
for more context). This HPA configuration explicitly says that each pod
of this application supports 10 units of the `ClusterScalingSchedule`
metric. Multiple applications can share the same
`ClusterScalingSchedule` or `ScalingSchedule` event and have a different
number of pods based on its `target.averageValue` configuration.

In our specific example at `2021-10-02T08:08:08+02:00` as the metric has
the value 100, this application will scale to 10 pods (100/10). Every
Monday, Wednesday and Friday, starting at 15 hours and 45 minutes
(Berlin time) the application will scale to 12 pods (120/10). Both
scaling up will last at least the configured duration times of the
schedules. After that, regular HPA scale down behavior applies.

Note that these number of pods are just considering these custom
metrics, the normal HPA behavior still applies, such as: in case of
multiple metrics the biggest number of pods is the utilized one, HPA max
and min replica configuration, autoscaling policies, etc.
