package collector

import (
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/zalando-incubator/kube-metrics-adapter/pkg/zmon"
	autoscalingv2beta1 "k8s.io/api/autoscaling/v2beta1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/metrics/pkg/apis/external_metrics"
)

const (
	// ZMONCheckMetric defines the metric name for metrics based on ZMON
	// checks.
	ZMONCheckMetric            = "zmon-check"
	zmonCheckIDLabelKey        = "check-id"
	zmonKeyLabelKey            = "key"
	zmonDurationLabelKey       = "duration"
	zmonAggregatorsLabelKey    = "aggregators"
	zmonTagPrefixLabelKey      = "tag-"
	defaultQueryDuration       = 10 * time.Minute
	zmonKeyAnnotationKey       = "metric-config.external.zmon-check.zmon/key"
	zmonTagPrefixAnnotationKey = "metric-config.external.zmon-check.zmon/tag-"
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
func (c *ZMONCollectorPlugin) NewCollector(hpa *autoscalingv2beta1.HorizontalPodAutoscaler, config *MetricConfig, interval time.Duration) (Collector, error) {
	switch config.Name {
	case ZMONCheckMetric:
		annotations := map[string]string{}
		if hpa != nil {
			annotations = hpa.Annotations
		}
		return NewZMONCollector(c.zmon, config, annotations, interval)
	}

	return nil, fmt.Errorf("metric '%s' not supported", config.Name)
}

// ZMONCollector defines a collector that is able to collect metrics from ZMON.
type ZMONCollector struct {
	zmon        zmon.ZMON
	interval    time.Duration
	checkID     int
	key         string
	labels      map[string]string
	tags        map[string]string
	duration    time.Duration
	aggregators []string
	metricName  string
	metricType  autoscalingv2beta1.MetricSourceType
}

// NewZMONCollector initializes a new ZMONCollector.
func NewZMONCollector(zmon zmon.ZMON, config *MetricConfig, annotations map[string]string, interval time.Duration) (*ZMONCollector, error) {
	checkIDStr, ok := config.Labels[zmonCheckIDLabelKey]
	if !ok {
		return nil, fmt.Errorf("ZMON check ID not specified on metric")
	}

	checkID, err := strconv.Atoi(checkIDStr)
	if err != nil {
		return nil, err
	}

	key := ""

	// get optional key
	if k, ok := config.Labels[zmonKeyLabelKey]; ok {
		key = k
	}

	// annotations takes precedence over label
	if k, ok := annotations[zmonKeyAnnotationKey]; ok {
		key = k
	}

	duration := defaultQueryDuration

	// parse optional duration value
	if d, ok := config.Labels[zmonDurationLabelKey]; ok {
		duration, err = time.ParseDuration(d)
		if err != nil {
			return nil, err
		}
	}

	// parse tags
	tags := make(map[string]string)
	for k, v := range config.Labels {
		if strings.HasPrefix(k, zmonTagPrefixLabelKey) {
			key := strings.TrimPrefix(k, zmonTagPrefixLabelKey)
			tags[key] = v
		}
	}

	// parse tags from annotations
	// tags defined in annotations takes precedence over tags defined in
	// the labels.
	for k, v := range annotations {
		if strings.HasPrefix(k, zmonTagPrefixAnnotationKey) {
			key := strings.TrimPrefix(k, zmonTagPrefixAnnotationKey)
			tags[key] = v
		}
	}

	// default aggregator is last
	aggregators := []string{"last"}
	if k, ok := config.Labels[zmonAggregatorsLabelKey]; ok {
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
		metricName:  config.Name,
		metricType:  config.Type,
		labels:      config.Labels,
	}, nil
}

// GetMetrics returns a list of collected metrics for the ZMON check.
func (c *ZMONCollector) GetMetrics() ([]CollectedMetric, error) {
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
		Type: c.metricType,
		External: external_metrics.ExternalMetricValue{
			MetricName:   c.metricName,
			MetricLabels: c.labels,
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
