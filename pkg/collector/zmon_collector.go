package collector

import (
	"context"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/zalando-incubator/kube-metrics-adapter/pkg/zmon"
	autoscalingv2 "k8s.io/api/autoscaling/v2"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/metrics/pkg/apis/external_metrics"
)

const (
	// ZMONMetricType defines the metric type for metrics based on ZMON
	// checks.
	ZMONMetricType          = "zmon"
	ZMONCheckMetricLegacy   = "zmon-check"
	zmonCheckIDLabelKey     = "check-id"
	zmonKeyLabelKey         = "key"
	zmonDurationLabelKey    = "duration"
	zmonAggregatorsLabelKey = "aggregators"
	zmonTagPrefixLabelKey   = "tag-"
	defaultQueryDuration    = 10 * time.Minute
)

// ZMONCollectorPlugin defines a plugin for creating collectors that can get
// metrics from ZMON.
type ZMONCollectorPlugin struct {
	zmon zmon.ZMON
}

// NewZMONCollectorPlugin initializes a new ZMONCollectorPlugin.
func NewZMONCollectorPlugin(zmon zmon.ZMON) (*ZMONCollectorPlugin, error) {
	return &ZMONCollectorPlugin{
		zmon: zmon,
	}, nil
}

// NewCollector initializes a new ZMON collector from the specified HPA.
func (c *ZMONCollectorPlugin) NewCollector(_ context.Context, hpa *autoscalingv2.HorizontalPodAutoscaler, config *MetricConfig, interval time.Duration) (Collector, error) {
	return NewZMONCollector(c.zmon, hpa, config, interval)
}

// ZMONCollector defines a collector that is able to collect metrics from ZMON.
type ZMONCollector struct {
	zmon        zmon.ZMON
	interval    time.Duration
	checkID     int
	key         string
	tags        map[string]string
	duration    time.Duration
	aggregators []string
	metric      autoscalingv2.MetricIdentifier
	metricType  autoscalingv2.MetricSourceType
	namespace   string
}

// NewZMONCollector initializes a new ZMONCollector.
func NewZMONCollector(zmon zmon.ZMON, hpa *autoscalingv2.HorizontalPodAutoscaler, config *MetricConfig, interval time.Duration) (*ZMONCollector, error) {
	if config.Metric.Selector == nil {
		return nil, fmt.Errorf("selector for zmon-check is not specified")
	}

	checkIDStr, ok := config.Config[zmonCheckIDLabelKey]
	if !ok {
		return nil, fmt.Errorf("ZMON check ID not specified on metric")
	}

	checkID, err := strconv.Atoi(checkIDStr)
	if err != nil {
		return nil, err
	}

	key := ""

	// get optional key
	if k, ok := config.Config[zmonKeyLabelKey]; ok {
		key = k
	}

	duration := defaultQueryDuration

	// parse optional duration value
	if d, ok := config.Config[zmonDurationLabelKey]; ok {
		duration, err = time.ParseDuration(d)
		if err != nil {
			return nil, err
		}
	}

	// parse tags
	tags := make(map[string]string)
	for k, v := range config.Config {
		if strings.HasPrefix(k, zmonTagPrefixLabelKey) {
			key := strings.TrimPrefix(k, zmonTagPrefixLabelKey)
			tags[key] = v
		}
	}

	// default aggregator is last
	aggregators := []string{"last"}
	if k, ok := config.Config[zmonAggregatorsLabelKey]; ok {
		aggregators = strings.Split(k, ",")
	}

	return &ZMONCollector{
		zmon:        zmon,
		interval:    interval,
		checkID:     checkID,
		key:         key,
		tags:        tags,
		duration:    duration,
		aggregators: aggregators,
		metric:      config.Metric,
		metricType:  config.Type,
		namespace:   hpa.Namespace,
	}, nil
}

// GetMetrics returns a list of collected metrics for the ZMON check.
func (c *ZMONCollector) GetMetrics(ctx context.Context) ([]CollectedMetric, error) {
	dataPoints, err := c.zmon.Query(c.checkID, c.key, c.tags, c.aggregators, c.duration)
	if err != nil {
		return nil, err
	}

	if len(dataPoints) < 1 {
		return nil, nil
	}

	// pick the last data point
	// TODO: do more fancy aggregations here (or in the query function)
	point := dataPoints[len(dataPoints)-1]

	metricValue := CollectedMetric{
		Namespace: c.namespace,
		Type:      c.metricType,
		External: external_metrics.ExternalMetricValue{
			MetricName:   c.metric.Name,
			MetricLabels: c.metric.Selector.MatchLabels,
			Timestamp:    metav1.Time{Time: point.Time},
			Value:        *resource.NewMilliQuantity(int64(point.Value*1000), resource.DecimalSI),
		},
	}

	return []CollectedMetric{metricValue}, nil
}

// Interval returns the interval at which the collector should run.
func (c *ZMONCollector) Interval() time.Duration {
	return c.interval
}
