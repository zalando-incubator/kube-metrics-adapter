package collector

import (
	"fmt"
	"strings"
	"time"

	autoscalingv2beta1 "k8s.io/api/autoscaling/v2beta1"
	"k8s.io/metrics/pkg/apis/custom_metrics"
	"k8s.io/metrics/pkg/apis/external_metrics"
)

const (
	customMetricsPrefix      = "metric-config."
	perReplicaMetricsConfKey = "per-replica"
	intervalMetricsConfKey   = "interval"
)

type ObjectReference struct {
	autoscalingv2beta1.CrossVersionObjectReference
	Namespace string
}

type CollectorFactory struct {
	podsPlugins     pluginMap
	objectPlugins   objectPluginMap
	externalPlugins map[string]CollectorPlugin
}

type objectPluginMap struct {
	Any   pluginMap
	Named map[string]*pluginMap
}

type pluginMap struct {
	Any   CollectorPlugin
	Named map[string]CollectorPlugin
}

func NewCollectorFactory() *CollectorFactory {
	return &CollectorFactory{
		podsPlugins: pluginMap{Named: map[string]CollectorPlugin{}},
		objectPlugins: objectPluginMap{
			Any:   pluginMap{},
			Named: map[string]*pluginMap{},
		},
		externalPlugins: map[string]CollectorPlugin{},
	}
}

type CollectorPlugin interface {
	NewCollector(hpa *autoscalingv2beta1.HorizontalPodAutoscaler, config *MetricConfig, interval time.Duration) (Collector, error)
}

func (c *CollectorFactory) RegisterPodsCollector(metricCollector string, plugin CollectorPlugin) error {
	if metricCollector == "" {
		c.podsPlugins.Any = plugin
	} else {
		c.podsPlugins.Named[metricCollector] = plugin
	}
	return nil

}

func (c *CollectorFactory) RegisterObjectCollector(kind, metricCollector string, plugin CollectorPlugin) error {
	if kind == "" {
		if metricCollector == "" {
			c.objectPlugins.Any.Any = plugin
		} else {
			if c.objectPlugins.Any.Named == nil {
				c.objectPlugins.Any.Named = map[string]CollectorPlugin{
					metricCollector: plugin,
				}
			} else {
				c.objectPlugins.Any.Named[metricCollector] = plugin
			}
		}
	} else {
		if named, ok := c.objectPlugins.Named[kind]; ok {
			if metricCollector == "" {
				named.Any = plugin
			} else {
				named.Named[metricCollector] = plugin
			}
		} else {
			if metricCollector == "" {
				c.objectPlugins.Named[kind] = &pluginMap{
					Any: plugin,
				}
			} else {
				c.objectPlugins.Named[kind] = &pluginMap{
					Named: map[string]CollectorPlugin{
						metricCollector: plugin,
					},
				}
			}
		}
	}

	return nil
}

func (c *CollectorFactory) RegisterExternalCollector(metrics []string, plugin CollectorPlugin) {
	for _, metric := range metrics {
		c.externalPlugins[metric] = plugin
	}
}

func (c *CollectorFactory) NewCollector(hpa *autoscalingv2beta1.HorizontalPodAutoscaler, config *MetricConfig, interval time.Duration) (Collector, error) {
	switch config.Type {
	case autoscalingv2beta1.PodsMetricSourceType:
		// first try to find a plugin by format
		if plugin, ok := c.podsPlugins.Named[config.CollectorName]; ok {
			return plugin.NewCollector(hpa, config, interval)
		}

		// else try to use the default plugin if set
		if c.podsPlugins.Any != nil {
			return c.podsPlugins.Any.NewCollector(hpa, config, interval)
		}
	case autoscalingv2beta1.ObjectMetricSourceType:
		// first try to find a plugin by kind
		if kinds, ok := c.objectPlugins.Named[config.ObjectReference.Kind]; ok {
			if plugin, ok := kinds.Named[config.CollectorName]; ok {
				return plugin.NewCollector(hpa, config, interval)
			}

			if kinds.Any != nil {
				return kinds.Any.NewCollector(hpa, config, interval)
			}
			break
		}

		// else try to find a default plugin for this kind
		if plugin, ok := c.objectPlugins.Any.Named[config.CollectorName]; ok {
			return plugin.NewCollector(hpa, config, interval)
		}

		if c.objectPlugins.Any.Any != nil {
			return c.objectPlugins.Any.Any.NewCollector(hpa, config, interval)
		}
	case autoscalingv2beta1.ExternalMetricSourceType:
		if plugin, ok := c.externalPlugins[config.Name]; ok {
			return plugin.NewCollector(hpa, config, interval)
		}
	}

	return nil, fmt.Errorf("no plugin found for %s", config.MetricTypeName)
}

func getObjectReference(hpa *autoscalingv2beta1.HorizontalPodAutoscaler, metricName string) (custom_metrics.ObjectReference, error) {
	for _, metric := range hpa.Spec.Metrics {
		if metric.Type == autoscalingv2beta1.ObjectMetricSourceType && metric.Object.MetricName == metricName {
			return custom_metrics.ObjectReference{
				APIVersion: metric.Object.Target.APIVersion,
				Kind:       metric.Object.Target.Kind,
				Name:       metric.Object.Target.Name,
				Namespace:  hpa.Namespace,
			}, nil
		}
	}

	return custom_metrics.ObjectReference{}, fmt.Errorf("failed to find object reference")
}

type MetricTypeName struct {
	Type autoscalingv2beta1.MetricSourceType
	Name string
}

type CollectedMetric struct {
	Type     autoscalingv2beta1.MetricSourceType
	Custom   custom_metrics.MetricValue
	External external_metrics.ExternalMetricValue
	Labels   map[string]string
}

type Collector interface {
	GetMetrics() ([]CollectedMetric, error)
	Interval() time.Duration
}

type MetricConfig struct {
	MetricTypeName
	CollectorName   string
	Config          map[string]string
	ObjectReference custom_metrics.ObjectReference
	PerReplica      bool
	Interval        time.Duration
	Labels          map[string]string
}

func parseCustomMetricsAnnotations(annotations map[string]string) (map[MetricTypeName]*MetricConfig, error) {
	metrics := make(map[MetricTypeName]*MetricConfig)
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

		metricTypeName := MetricTypeName{
			Name: configs[2],
		}

		switch configs[1] {
		case "pods":
			metricTypeName.Type = autoscalingv2beta1.PodsMetricSourceType
		case "object":
			metricTypeName.Type = autoscalingv2beta1.ObjectMetricSourceType
		}

		metricCollector := configs[3]

		config, ok := metrics[metricTypeName]
		if !ok {
			config = &MetricConfig{
				MetricTypeName: metricTypeName,
				CollectorName:  metricCollector,
				Config:         map[string]string{},
			}
			metrics[metricTypeName] = config
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
				return nil, fmt.Errorf("failed to parse interval value %s for %s: %v", val, key, err)
			}
			config.Interval = interval
			continue
		}

		config.Config[parts[1]] = val
	}

	return metrics, nil
}

// ParseHPAMetrics parses the HPA object into a list of metric configurations.
func ParseHPAMetrics(hpa *autoscalingv2beta1.HorizontalPodAutoscaler) ([]*MetricConfig, error) {
	metricConfigs := make([]*MetricConfig, 0, len(hpa.Spec.Metrics))

	// TODO: validate that the specified metric names are defined
	// in the HPA
	configs, err := parseCustomMetricsAnnotations(hpa.Annotations)
	if err != nil {
		return nil, err
	}

	for _, metric := range hpa.Spec.Metrics {
		typeName := MetricTypeName{
			Type: metric.Type,
		}

		var ref custom_metrics.ObjectReference
		switch metric.Type {
		case autoscalingv2beta1.PodsMetricSourceType:
			typeName.Name = metric.Pods.MetricName
		case autoscalingv2beta1.ObjectMetricSourceType:
			typeName.Name = metric.Object.MetricName
			ref = custom_metrics.ObjectReference{
				APIVersion: metric.Object.Target.APIVersion,
				Kind:       metric.Object.Target.Kind,
				Name:       metric.Object.Target.Name,
				Namespace:  hpa.Namespace,
			}
		case autoscalingv2beta1.ExternalMetricSourceType:
			typeName.Name = metric.External.MetricName
		case autoscalingv2beta1.ResourceMetricSourceType:
			continue // kube-metrics-adapter does not collect resource metrics
		}

		if config, ok := configs[typeName]; ok {
			config.ObjectReference = ref
			metricConfigs = append(metricConfigs, config)
			continue
		}

		config := &MetricConfig{
			MetricTypeName:  typeName,
			ObjectReference: ref,
			Config:          map[string]string{},
		}

		if metric.Type == autoscalingv2beta1.ExternalMetricSourceType {
			config.Labels = metric.External.MetricSelector.MatchLabels
		}
		metricConfigs = append(metricConfigs, config)
	}

	return metricConfigs, nil
}
