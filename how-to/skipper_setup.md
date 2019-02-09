# Skipper Prometheus Metrics Collection

The skipper-ingress pods should be configured to be scraped by Prometheus. This
can be done by Prometheus service discovery using discovery of Kubernetes services
or Kubernetes pods:

```yaml
apiVersion: v1
kind: Service
metadata:
  annotations:
    prometheus.io/path: /metrics
    prometheus.io/port: "9911"
    prometheus.io/scrape: "true"
  labels:
    application: skipper-ingress
  name: skipper-ingress
spec:
  ports:
  - port: 80
    protocol: TCP
    targetPort: 9999
  selector:
    application: skipper-ingress
  type: ClusterIP
```
This [configuration](https://github.com/zalando-incubator/kubernetes-on-aws/blob/dev/cluster/manifests/prometheus/configmap.yaml#L69)
shows how prometheus is configured in our clusters to scrape service endpoints.
The annotations `prometheus.io/path`, `prometheus.io/port` and `prometheus.io/scrape`
instruct Prometheus to scrape all pods of this service on the port _9911_ and
the path `/metrics`.

When the `kube-metrics-adapter` is started the flag `--prometheus-server` should be set so that
the adapter can query prometheus to get aggregated metrics. When running in kubernetes it can
be the service address of the prometheus service like `http://prometheus.kube-system`.

With these settings the `kube-metrics-adapter` can provide `request-per-second` metrics for ingress
objects which are present in the cluster. The prometheus instances scrape the metrics from
the `skipper-ingress` pods. The adapter then queries prometheus to get the metric and then
provides them to the API server when requested.
