package collector

import (
	"fmt"
	"net/url"
	"time"

	"github.com/zalando-incubator/kube-metrics-adapter/pkg/collector/httpmetrics"

	"k8s.io/api/autoscaling/v2beta2"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/metrics/pkg/apis/external_metrics"
)

const (
	HTTPJSONPathType          = "json-path"
	HTTPMetricNameLegacy      = "http"
	HTTPEndpointAnnotationKey = "endpoint"
	HTTPJsonPathAnnotationKey = "json-key"
	identifierLabel           = "identifier"
)

type HTTPCollectorPlugin struct{}

func NewHTTPCollectorPlugin() (*HTTPCollectorPlugin, error) {
	return &HTTPCollectorPlugin{}, nil
}

func (p *HTTPCollectorPlugin) NewCollector(hpa *v2beta2.HorizontalPodAutoscaler, config *MetricConfig, interval time.Duration) (Collector, error) {
	collector := &HTTPCollector{
		namespace: hpa.Namespace,
	}
	var (
		value string
		ok    bool
	)
	if value, ok = config.Config[HTTPJsonPathAnnotationKey]; !ok {
		return nil, fmt.Errorf("config value %s not found", HTTPJsonPathAnnotationKey)
	}
	jsonPath := value

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
	if _, ok := config.Metric.Selector.MatchLabels[identifierLabel]; !ok {
		return nil, fmt.Errorf("%s is not specified as a label for metric %s", identifierLabel, config.Metric.Name)
	}
	collector.metric = config.Metric
	var aggFunc httpmetrics.AggregatorFunc

	if val, ok := config.Config["aggregator"]; ok {
		aggFunc, err = httpmetrics.ParseAggregator(val)
		if err != nil {
			return nil, err
		}
	}
	jsonPathGetter, err := httpmetrics.NewJSONPathMetricsGetter(httpmetrics.DefaultMetricsHTTPClient(), aggFunc, jsonPath)
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
	metricType    v2beta2.MetricSourceType
	metricsGetter *httpmetrics.JSONPathMetricsGetter
	metric        v2beta2.MetricIdentifier
}

func (c *HTTPCollector) GetMetrics() ([]CollectedMetric, error) {
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
