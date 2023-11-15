package collector

import (
	"context"
	"fmt"
	"time"

	"github.com/zalando-incubator/kube-metrics-adapter/pkg/nakadi"
	autoscalingv2 "k8s.io/api/autoscaling/v2"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/metrics/pkg/apis/external_metrics"
)

const (
	// NakadiMetricType defines the metric type for metrics based on Nakadi
	// subscriptions.
	NakadiMetricType                   = "nakadi"
	nakadiSubscriptionIDKey            = "subscription-id"
	nakadiMetricTypeKey                = "metric-type"
	nakadiMetricTypeConsumerLagSeconds = "consumer-lag-seconds"
	nakadiMetricTypeUnconsumedEvents   = "unconsumed-events"
)

// NakadiCollectorPlugin defines a plugin for creating collectors that can get
// unconsumed events from Nakadi.
type NakadiCollectorPlugin struct {
	nakadi nakadi.Nakadi
}

// NewNakadiCollectorPlugin initializes a new NakadiCollectorPlugin.
func NewNakadiCollectorPlugin(nakadi nakadi.Nakadi) (*NakadiCollectorPlugin, error) {
	return &NakadiCollectorPlugin{
		nakadi: nakadi,
	}, nil
}

// NewCollector initializes a new Nakadi collector from the specified HPA.
func (c *NakadiCollectorPlugin) NewCollector(hpa *autoscalingv2.HorizontalPodAutoscaler, config *MetricConfig, interval time.Duration) (Collector, error) {
	return NewNakadiCollector(c.nakadi, hpa, config, interval)
}

// NakadiCollector defines a collector that is able to collect metrics from
// Nakadi.
type NakadiCollector struct {
	nakadi           nakadi.Nakadi
	interval         time.Duration
	subscriptionID   string
	nakadiMetricType string
	metric           autoscalingv2.MetricIdentifier
	metricType       autoscalingv2.MetricSourceType
	namespace        string
}

// NewNakadiCollector initializes a new NakadiCollector.
func NewNakadiCollector(nakadi nakadi.Nakadi, hpa *autoscalingv2.HorizontalPodAutoscaler, config *MetricConfig, interval time.Duration) (*NakadiCollector, error) {
	if config.Metric.Selector == nil {
		return nil, fmt.Errorf("selector for nakadi is not specified")
	}

	subscriptionID, ok := config.Config[nakadiSubscriptionIDKey]
	if !ok {
		return nil, fmt.Errorf("subscription-id not specified on metric")
	}

	metricType, ok := config.Config[nakadiMetricTypeKey]
	if !ok {
		return nil, fmt.Errorf("metric-type not specified on metric")
	}

	if metricType != nakadiMetricTypeConsumerLagSeconds && metricType != nakadiMetricTypeUnconsumedEvents {
		return nil, fmt.Errorf("metric-type must be either '%s' or '%s', was '%s'", nakadiMetricTypeConsumerLagSeconds, nakadiMetricTypeUnconsumedEvents, metricType)
	}

	return &NakadiCollector{
		nakadi:           nakadi,
		interval:         interval,
		subscriptionID:   subscriptionID,
		nakadiMetricType: metricType,
		metric:           config.Metric,
		metricType:       config.Type,
		namespace:        hpa.Namespace,
	}, nil
}

// GetMetrics returns a list of collected metrics for the Nakadi subscription ID.
func (c *NakadiCollector) GetMetrics() ([]CollectedMetric, error) {
	var value int64
	var err error
	switch c.nakadiMetricType {
	case nakadiMetricTypeConsumerLagSeconds:
		value, err = c.nakadi.ConsumerLagSeconds(context.TODO(), c.subscriptionID)
		if err != nil {
			return nil, err
		}
	case nakadiMetricTypeUnconsumedEvents:
		value, err = c.nakadi.UnconsumedEvents(context.TODO(), c.subscriptionID)
		if err != nil {
			return nil, err
		}
	}

	metricValue := CollectedMetric{
		Namespace: c.namespace,
		Type:      c.metricType,
		External: external_metrics.ExternalMetricValue{
			MetricName:   c.metric.Name,
			MetricLabels: c.metric.Selector.MatchLabels,
			Timestamp:    metav1.Now(),
			Value:        *resource.NewQuantity(value, resource.DecimalSI),
		},
	}

	return []CollectedMetric{metricValue}, nil
}

// Interval returns the interval at which the collector should run.
func (c *NakadiCollector) Interval() time.Duration {
	return c.interval
}
