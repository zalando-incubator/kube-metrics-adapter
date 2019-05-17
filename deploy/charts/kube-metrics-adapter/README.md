# Kube Metrics Adapter

Installs the [# Kube Metrics Adapter](https://github.com/banzaicloud/kube-metrics-adapter/) for the Custom Metrics API. Custom metrics are used in Kubernetes by [Horizontal Pod Autoscalers](https://kubernetes.io/docs/tasks/run-application/horizontal-pod-autoscale/) to scale workloads based upon your own metric pulled from an external metrics provider like Prometheus. This chart complements the [metrics-server](https://github.com/helm/charts/tree/master/stable/metrics-server) chart that provides resource only metrics.

## Prerequisites

Kubernetes 1.9+

## Installing the Chart

To install the chart with the release name `my-release`:

```console
$ helm install --name my-release stable/kube-metrics-adapter
```

This command deploys the kube-metrics-adapter with the default configuration. The [configuration](#configuration) section lists the parameters that can be configured during installation.

## Using the Chart

[kube-metrics-adapter](https://github.com/banzaicloud/kube-metrics-adapter) can be configure to use several different collectors. Currently this chart supports only configuration of Prometheus collector. Ensure the `prometheus.url` and `prometheus.port` are configured with the correct Prometheus service endpoint. To configure your Horizontal Pod Autoscaler to use the custom metric, see the custom metrics section of the [HPA walkthrough](https://kubernetes.io/docs/tasks/run-application/horizontal-pod-autoscale-walkthrough/#autoscaling-on-multiple-metrics-and-custom-metrics).

## Uninstalling the Chart

To uninstall/delete the `my-release` deployment:

```console
$ helm delete my-release
```

The command removes all the Kubernetes components associated with the chart and deletes the release.

## Configuration

The following table lists the configurable parameters of the Prometheus Adapter chart and their default values.

| Parameter                       | Description                                                                     | Default                                     |
| ------------------------------- | ------------------------------------------------------------------------------- | --------------------------------------------|
| `affinity`                      | Node affinity                                                                   | `{}`                                        |
| `image.repository`              | Image repository                                                                | `banzaicloud/kube-metrics-adapter`          |
| `image.tag`                     | Image tag                                                                       | `0.1.1`                                     |
| `image.pullPolicy`              | Image pull policy                                                               | `IfNotPresent`                              |
| `image.pullSecrets`             | Image pull secrets                                                              | `{}`                                        |
| `logLevel`                      | Log level                                                                       | `4`                                         |
| `nodeSelector`                  | Node labels for pod assignment                                                  | `{}`                                        |
| `prometheus.url`                | Url of where we can find the Prometheus service                                 | `http://prometheus.default.svc`             |
| `rbac.create`                   | If true, create & use RBAC resources                                            | `true`                                      |
| `resources`                     | CPU/Memory resource requests/limits                                             | `{}`                                        |                                                                                                        
| `service.annotations`           | Annotations to add to the service                                               | `{}`                                        |
| `service.port`                  | Service port to expose                                                          | `443`                                       |
| `service.internalPort`          | Service internal port                                                           | `6443`                                      |
| `service.type`                  | Type of service to create                                                       | `ClusterIP`                                 |
| `serviceAccount.create`         | If true, create & use Serviceaccount                                            | `true`                                      |
| `serviceAccount.name`           | If not set and create is true, a name is generated using the fullname template  | ``                                          |
| `sslCertPath`                   | Path on the pod where ssl ca cert exists (required if aws.enable=true)          | `/etc/ssl/certs/ca-certificates.crt`        |
| `sslCertHostPath`               | Path on the host where ssl ca cert exists (required if aws.enable=true)         | `/etc/ssl/certs/ca-certificates.crt`        |
| `tls.enable`                    | If true, use the provided certificates. If false, generate self-signed certs    | `false`                                     |
| `tls.ca`                        | Public CA file that signed the APIService (ignored if tls.enable=false)         | ``                                          |
| `tls.key`                       | Private key of the APIService (ignored if tls.enable=false)                     | ``                                          |
| `tls.certificate`               | Public key of the APIService (ignored if tls.enable=false)                      | ``                                          |
| `aws.enable`                    | If true, enable AWS external metrics (SQS)                                      | `false`                                     |
| `aws.region`                    | Comma separated list of AWS regions (ignored if aws.enable=false)               | `us-west-2`                                 |
| `tolerations`                   | List of node taints to tolerate                                                 | `[]`                                        |
| `pspEnabled`                    | enabel PSP resources                                                            | false                                       |
Specify each parameter using the `--set key=value[,key=value]` argument to `helm install`. For example,

```console
$ helm install --name my-release \
  --set logLevel=1 \
 stable/kube-metrics-adapter
```

Alternatively, a YAML file that specifies the values for the above parameters can be provided while installing the chart. For example,

```console
$ helm install --name my-release -f values.yaml stable/kube-metrics-adapter
```
