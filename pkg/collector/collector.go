package collector

import (
	"fmt"
	"time"

	"github.com/zalando-incubator/kube-metrics-adapter/pkg/annotations"
	autoscalingv2 "k8s.io/api/autoscaling/v2beta2"
	"k8s.io/metrics/pkg/apis/custom_metrics"
	"k8s.io/metrics/pkg/apis/external_metrics"
)

type ObjectReference struct {
	autoscalingv2.CrossVersionObjectReference
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
	NewCollector(hpa *autoscalingv2.HorizontalPodAutoscaler, config *MetricConfig, interval time.Duration) (Collector, error)
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

func (c *CollectorFactory) NewCollector(hpa *autoscalingv2.HorizontalPodAutoscaler, config *MetricConfig, interval time.Duration) (Collector, error) {
	switch config.Type {
	case autoscalingv2.PodsMetricSourceType:
		// first try to find a plugin by format
		if plugin, ok := c.podsPlugins.Named[config.CollectorName]; ok {
			return plugin.NewCollector(hpa, config, interval)
		}

		// else try to use the default plugin if set
		if c.podsPlugins.Any != nil {
			return c.podsPlugins.Any.NewCollector(hpa, config, interval)
		}
	case autoscalingv2.ObjectMetricSourceType:
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
	case autoscalingv2.ExternalMetricSourceType:
		if plugin, ok := c.externalPlugins[config.Metric.Name]; ok {
			return plugin.NewCollector(hpa, config, interval)
		}
	}

	return nil, fmt.Errorf("no plugin found for %s", config.MetricTypeName)
}

type MetricTypeName struct {
	Type   autoscalingv2.MetricSourceType
	Metric autoscalingv2.MetricIdentifier
}

type CollectedMetric struct {
	Type     autoscalingv2.MetricSourceType
	Custom   custom_metrics.MetricValue
	External external_metrics.ExternalMetricValue
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
	Configuration   map[string]string
}

// ParseHPAMetrics parses the HPA object into a list of metric configurations.
func ParseHPAMetrics(hpa *autoscalingv2.HorizontalPodAutoscaler) ([]*MetricConfig, error) {
	metricConfigs := make([]*MetricConfig, 0, len(hpa.Spec.Metrics))

	// TODO: validate that the specified metric names are defined
	// in the HPA
	var parser annotations.AnnotationConfigMap
	err := parser.Parse(hpa.Annotations)
	if err != nil {
		return nil, err
	}

	for _, metric := range hpa.Spec.Metrics {
		typeName := MetricTypeName{
			Type: metric.Type,
		}

		var ref custom_metrics.ObjectReference
		switch metric.Type {
		case autoscalingv2.PodsMetricSourceType:
			typeName.Metric = metric.Pods.Metric
		case autoscalingv2.ObjectMetricSourceType:
			typeName.Metric = metric.Object.Metric
			ref = custom_metrics.ObjectReference{
				APIVersion: metric.Object.DescribedObject.APIVersion,
				Kind:       metric.Object.DescribedObject.Kind,
				Name:       metric.Object.DescribedObject.Name,
				Namespace:  hpa.Namespace,
			}
		case autoscalingv2.ExternalMetricSourceType:
			typeName.Metric = metric.External.Metric
		case autoscalingv2.ResourceMetricSourceType:
			continue // kube-metrics-adapter does not collect resource metrics
		}

		config := &MetricConfig{
			MetricTypeName:  typeName,
			ObjectReference: ref,
			Config:          map[string]string{},
		}

		annotationConfigs, present := parser.GetAnnotationConfig(typeName.Metric.Name, typeName.Type)
		if present {
			config.CollectorName = annotationConfigs.CollectorName
			config.Interval = annotationConfigs.Interval
			config.PerReplica = annotationConfigs.PerReplica
			config.Config = annotationConfigs.Configs
		}
		metricConfigs = append(metricConfigs, config)
	}
	return metricConfigs, nil
}
