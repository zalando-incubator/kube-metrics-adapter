package collector

import (
	"context"
	"fmt"
	"net/url"
	"time"

	"github.com/zalando-incubator/kube-metrics-adapter/pkg/collector/httpmetrics"

	autoscalingv2 "k8s.io/api/autoscaling/v2"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/metrics/pkg/apis/external_metrics"
)

const (
	HTTPJSONPathType          = "json-path"
	HTTPMetricNameLegacy      = "http"
	HTTPEndpointAnnotationKey = "endpoint"
	HTTPJsonPathAnnotationKey = "json-key"
	HTTPJsonEvalAnnotationKey = "json-eval"
)

type HTTPCollectorPlugin struct{}

func NewHTTPCollectorPlugin() (*HTTPCollectorPlugin, error) {
	return &HTTPCollectorPlugin{}, nil
}

func (p *HTTPCollectorPlugin) NewCollector(_ context.Context, hpa *autoscalingv2.HorizontalPodAutoscaler, config *MetricConfig, interval time.Duration) (Collector, error) {
	collector := &HTTPCollector{
		namespace: hpa.Namespace,
	}

	var (
		value string
		ok    bool

		jsonPath string
		jsonEval string
	)

	if value, ok = config.Config[HTTPJsonPathAnnotationKey]; ok {
		jsonPath = value
	}
	if value, ok = config.Config[HTTPJsonEvalAnnotationKey]; ok {
		jsonEval = value
	}
	if jsonPath == "" && jsonEval == "" {
		return nil, fmt.Errorf("config value %s or %s not found", HTTPJsonPathAnnotationKey, HTTPJsonEvalAnnotationKey)
	}
	if jsonPath != "" && jsonEval != "" {
		return nil, fmt.Errorf("config value %s and %s cannot be used together", HTTPJsonPathAnnotationKey, HTTPJsonEvalAnnotationKey)
	}

	if value, ok = config.Config[HTTPEndpointAnnotationKey]; !ok {
		return nil, fmt.Errorf("config value %s not found", HTTPEndpointAnnotationKey)
	}
	var err error
	collector.endpoint, err = url.Parse(value)
	if err != nil {
		return nil, err
	}
	collector.interval = interval
	collector.metricType = config.Type
	if config.Metric.Selector == nil || config.Metric.Selector.MatchLabels == nil {
		return nil, fmt.Errorf("no label selector specified for metric: %s", config.Metric.Name)
	}
	collector.metric = config.Metric
	var aggFunc httpmetrics.AggregatorFunc

	if val, ok := config.Config["aggregator"]; ok {
		aggFunc, err = httpmetrics.ParseAggregator(val)
		if err != nil {
			return nil, err
		}
	}
	jsonPathGetter, err := httpmetrics.NewJSONPathMetricsGetter(httpmetrics.DefaultMetricsHTTPClient(), aggFunc, jsonPath, jsonEval)
	if err != nil {
		return nil, err
	}
	collector.metricsGetter = jsonPathGetter
	return collector, nil
}

type HTTPCollector struct {
	endpoint      *url.URL
	interval      time.Duration
	namespace     string
	metricType    autoscalingv2.MetricSourceType
	metricsGetter *httpmetrics.JSONPathMetricsGetter
	metric        autoscalingv2.MetricIdentifier
}

func (c *HTTPCollector) GetMetrics(ctx context.Context) ([]CollectedMetric, error) {
	metric, err := c.metricsGetter.GetMetric(*c.endpoint)
	if err != nil {
		return nil, err
	}

	value := CollectedMetric{
		Namespace: c.namespace,
		Type:      c.metricType,
		External: external_metrics.ExternalMetricValue{
			MetricName:   c.metric.Name,
			MetricLabels: c.metric.Selector.MatchLabels,
			Timestamp: metav1.Time{
				Time: time.Now(),
			},
			Value: *resource.NewMilliQuantity(int64(metric*1000), resource.DecimalSI),
		},
	}
	return []CollectedMetric{value}, nil
}

func (c *HTTPCollector) Interval() time.Duration {
	return c.interval
}
