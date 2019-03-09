package annotations

import (
	"fmt"
	autoscalingv2 "k8s.io/api/autoscaling/v2beta2"
	"strings"
	"time"
)

const (
	customMetricsPrefix      = "metric-config."
	perReplicaMetricsConfKey = "per-replica"
	intervalMetricsConfKey   = "interval"
)

type AnnotationConfigs struct {
	CollectorName string
	Configs       map[string]string
	PerReplica    bool
	Interval      time.Duration
}

type MetricConfigKey struct {
	Type       autoscalingv2.MetricSourceType
	MetricName string
}

type AnnotationConfigMap map[MetricConfigKey]*AnnotationConfigs

func (m AnnotationConfigMap) Parse(annotations map[string]string) error {
	for key, val := range annotations {
		if !strings.HasPrefix(key, customMetricsPrefix) {
			continue
		}

		parts := strings.Split(key, "/")
		if len(parts) != 2 {
			// TODO: error?
			continue
		}

		configs := strings.Split(parts[0], ".")
		if len(configs) != 4 {
			// TODO: error?
			continue
		}

		key := MetricConfigKey{
			MetricName: configs[2],
		}

		switch configs[1] {
		case "pods":
			key.Type = autoscalingv2.PodsMetricSourceType
		case "object":
			key.Type = autoscalingv2.ObjectMetricSourceType
		default:
			key.Type = autoscalingv2.ExternalMetricSourceType
		}

		metricCollector := configs[3]

		config, ok := m[key]
		if !ok {
			config = &AnnotationConfigs{
				CollectorName: metricCollector,
				Configs:       map[string]string{},
			}
			m[key] = config
		}

		// TODO: fail if collector name doesn't match
		if config.CollectorName != metricCollector {
			continue
		}

		if parts[1] == perReplicaMetricsConfKey {
			config.PerReplica = true
			continue
		}

		if parts[1] == intervalMetricsConfKey {
			interval, err := time.ParseDuration(val)
			if err != nil {
				return fmt.Errorf("failed to parse interval value %s for %s: %v", val, key, err)
			}
			config.Interval = interval
			continue
		}

		config.Configs[parts[1]] = val
	}
	return nil
}

func (m AnnotationConfigMap) GetAnnotationConfig(metricName string, metricType autoscalingv2.MetricSourceType) (*AnnotationConfigs, bool) {
	key := MetricConfigKey{MetricName: metricName, Type: metricType}
	config, ok := m[key]
	return config, ok
}
