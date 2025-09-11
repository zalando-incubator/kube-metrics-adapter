package collector

import (
	"context"
	"fmt"
	"math"
	"net/http"
	"time"

	"github.com/prometheus/client_golang/api"
	promv1 "github.com/prometheus/client_golang/api/prometheus/v1"
	promconfig "github.com/prometheus/common/config"
	"github.com/prometheus/common/model"
	autoscalingv2 "k8s.io/api/autoscaling/v2"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/metrics/pkg/apis/custom_metrics"
	"k8s.io/metrics/pkg/apis/external_metrics"
)

const (
	PrometheusMetricType               = "prometheus"
	PrometheusMetricNameLegacy         = "prometheus-query"
	prometheusQueryNameLabelKey        = "query-name"
	prometheusServerAnnotationKey      = "prometheus-server"
	prometheusServerAliasAnnotationKey = "prometheus-server-alias"
)

type NoResultError struct {
	query string
}

func (r NoResultError) Error() string {
	return fmt.Sprintf("query '%s' did not result a valid response", r.query)
}

type PrometheusCollectorPlugin struct {
	promAPI           promv1.API
	client             kubernetes.Interface
	additionalPromAPIs map[string]promv1.API
}

func getPrometheusAPI(prometheusServer, tokenFile string) (promv1.API, error) {
	roundTripper := http.DefaultTransport

	// If a token file is specified, use it for authentication.
	if tokenFile != "" {
		roundTripper = promconfig.NewAuthorizationCredentialsRoundTripper(
			"Bearer",
			promconfig.NewFileSecret(tokenFile),
			roundTripper,
		)
	}

	cfg := api.Config{
		Address:      prometheusServer,
		RoundTripper: roundTripper,
	}

	promClient, err := api.NewClient(cfg)
	if err != nil {
		return nil, err
	}

	return promv1.NewAPI(promClient), nil
}

func NewPrometheusCollectorPlugin(client kubernetes.Interface, prometheusServer, tokenFile string, additionalServers, additionalServerTokenFiles map[string]string) (*PrometheusCollectorPlugin, error) {
	promAPI, err := getPrometheusAPI(prometheusServer, tokenFile)
	if err != nil {
		return nil, err
	}

	additionalPromAPIs := make(map[string]promv1.API)

	for alias, server := range additionalServers {
		additionalPromAPIs[alias], err = getPrometheusAPI(server, additionalServerTokenFiles[alias])
		if err != nil {
			return nil, err
		}
	}

	return &PrometheusCollectorPlugin{
		client:             client,
		promAPI:            promAPI,
		additionalPromAPIs: additionalPromAPIs,
	}, nil
}

func (p *PrometheusCollectorPlugin) NewCollector(_ context.Context, hpa *autoscalingv2.HorizontalPodAutoscaler, config *MetricConfig, interval time.Duration) (Collector, error) {
	return NewPrometheusCollector(p.client, p.promAPI, p.additionalPromAPIs, hpa, config, interval)
}

type PrometheusCollector struct {
	client          kubernetes.Interface
	promAPI         promv1.API
	query           string
	metric          autoscalingv2.MetricIdentifier
	metricType      autoscalingv2.MetricSourceType
	objectReference custom_metrics.ObjectReference
	interval        time.Duration
	perReplica      bool
	hpa             *autoscalingv2.HorizontalPodAutoscaler
}

func NewPrometheusCollector(client kubernetes.Interface, promAPI promv1.API, additionalPromAPIs map[string]promv1.API, hpa *autoscalingv2.HorizontalPodAutoscaler, config *MetricConfig, interval time.Duration) (*PrometheusCollector, error) {
	c := &PrometheusCollector{
		client:     client,
		promAPI:    promAPI,
		interval:   interval,
		hpa:        hpa,
		metric:     config.Metric,
		metricType: config.Type,
	}

	switch config.Type {
	case autoscalingv2.ObjectMetricSourceType:
		c.objectReference = config.ObjectReference
		c.perReplica = config.PerReplica

		if v, ok := config.Config["query"]; ok {
			// TODO: validate query
			c.query = v
		} else {
			return nil, fmt.Errorf("no prometheus query defined")
		}
	case autoscalingv2.ExternalMetricSourceType:
		if config.Metric.Selector == nil {
			return nil, fmt.Errorf("selector for prometheus query is not specified")
		}

		if v, ok := config.Config["query"]; ok {
			// TODO: validate query
			c.query = v
		} else {
			// support legacy behavior of mapping query name to metric
			queryName, ok := config.Config[prometheusQueryNameLabelKey]
			if !ok {
				return nil, fmt.Errorf("query or query name not specified on metric")
			}

			if v, ok := config.Config[queryName]; ok {
				// TODO: validate query
				c.query = v
			} else {
				return nil, fmt.Errorf("no prometheus query defined for metric")
			}
		}

		// Use custom Prometheus URL if defined in HPA annotation.
		if promServer, ok := config.Config[prometheusServerAnnotationKey]; ok {
			cfg := api.Config{
				Address:      promServer,
				RoundTripper: http.DefaultTransport,
			}

			promClient, err := api.NewClient(cfg)
			if err != nil {
				return nil, err
			}
			c.promAPI = promv1.NewAPI(promClient)
		} else if promServerAlias, ok := config.Config[prometheusServerAliasAnnotationKey]; ok {
			if promAPI, ok := additionalPromAPIs[promServerAlias]; ok {
				c.promAPI = promAPI
			} else {
				return nil, fmt.Errorf("no additional prometheus server found for alias %s", promServerAlias)
			}
		}
	}

	return c, nil
}

func (c *PrometheusCollector) GetMetrics(ctx context.Context) ([]CollectedMetric, error) {
	// TODO: use real context
	value, _, err := c.promAPI.Query(ctx, c.query, time.Now().UTC())
	if err != nil {
		return nil, err
	}

	var sampleValue model.SampleValue
	switch value.Type() {
	case model.ValVector:
		samples := value.(model.Vector)
		if len(samples) == 0 {
			return nil, &NoResultError{query: c.query}
		}

		sampleValue = samples[0].Value
	case model.ValScalar:
		scalar := value.(*model.Scalar)
		sampleValue = scalar.Value
	}

	if math.IsNaN(float64(sampleValue)) {
		return nil, &NoResultError{query: c.query}
	}

	if c.perReplica {
		// get current replicas for the targeted scale object. This is used to
		// calculate an average metric instead of total.
		// targetAverageValue will be available in Kubernetes v1.12
		// https://github.com/kubernetes/kubernetes/pull/64097
		replicas, err := targetRefReplicas(ctx, c.client, c.hpa)
		if err != nil {
			return nil, err
		}
		sampleValue = model.SampleValue(float64(sampleValue) / float64(replicas))
	}

	var metricValue CollectedMetric
	switch c.metricType {
	case autoscalingv2.ObjectMetricSourceType:
		metricValue = CollectedMetric{
			Namespace: c.hpa.Namespace,
			Type:      c.metricType,
			Custom: custom_metrics.MetricValue{
				DescribedObject: c.objectReference,
				Metric:          custom_metrics.MetricIdentifier{Name: c.metric.Name, Selector: c.metric.Selector},
				Timestamp:       metav1.Time{Time: time.Now().UTC()},
				Value:           *resource.NewMilliQuantity(int64(sampleValue*1000), resource.DecimalSI),
			},
		}
	case autoscalingv2.ExternalMetricSourceType:
		metricValue = CollectedMetric{
			Namespace: c.hpa.Namespace,
			Type:      c.metricType,
			External: external_metrics.ExternalMetricValue{
				MetricName:   c.metric.Name,
				MetricLabels: c.metric.Selector.MatchLabels,
				Timestamp:    metav1.Time{Time: time.Now().UTC()},
				Value:        *resource.NewMilliQuantity(int64(sampleValue*1000), resource.DecimalSI),
			},
		}
	}

	return []CollectedMetric{metricValue}, nil
}

func (c *PrometheusCollector) Interval() time.Duration {
	return c.interval
}
