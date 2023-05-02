package collector

import (
	"time"

	autoscalingv2 "k8s.io/api/autoscaling/v2beta2"
	"k8s.io/apimachinery/pkg/api/resource"
	"k8s.io/metrics/pkg/apis/custom_metrics"
)

type FakeCollectorPlugin struct {
	metrics []CollectedMetric
	config  map[string]string
}

type FakeCollector struct {
	metrics  []CollectedMetric
	interval time.Duration
	stub     func() ([]CollectedMetric, error)
}

func (c *FakeCollector) GetMetrics() ([]CollectedMetric, error) {
	if c.stub != nil {
		v, err := c.stub()
		return v, err
	}

	return c.metrics, nil
}

func (FakeCollector) Interval() time.Duration {
	return time.Minute
}

func (p *FakeCollectorPlugin) NewCollector(
	hpa *autoscalingv2.HorizontalPodAutoscaler,
	config *MetricConfig,
	interval time.Duration,
) (Collector, error) {

	p.config = config.Config
	return &FakeCollector{metrics: p.metrics, interval: interval}, nil
}

func makePlugin(metric int) *FakeCollectorPlugin {
	return &FakeCollectorPlugin{
		metrics: []CollectedMetric{
			{
				Custom: custom_metrics.MetricValue{Value: *resource.NewQuantity(int64(metric), resource.DecimalSI)},
			},
		},
	}
}

func makeCollectorWithStub(f func() ([]CollectedMetric, error)) *FakeCollector {
	return &FakeCollector{stub: f}
}
