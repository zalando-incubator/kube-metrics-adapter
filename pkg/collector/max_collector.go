package collector

import (
	"k8s.io/apimachinery/pkg/api/resource"
	"time"
)

// MaxCollector is a simple aggregator collector that returns the maximum value
// of metrics from all collectors.
type WeightedMaxCollector struct {
	collectors []Collector
	interval   time.Duration
	weight     float64
}

// NewMaxCollector initializes a new MacCollector.
func NewWeightedMaxCollector(interval time.Duration, weight float64, collectors ...Collector) *WeightedMaxCollector {
	return &WeightedMaxCollector{
		collectors: collectors,
		interval:   interval,
		weight:     weight,
	}
}

// GetMetrics gets metrics from all collectors and return the higest value.
func (c *WeightedMaxCollector) GetMetrics() ([]CollectedMetric, error) {
	var max CollectedMetric
	for _, collector := range c.collectors {
		values, err := collector.GetMetrics()
		if err != nil {
			return nil, err
		}

		for _, value := range values {
			if value.Custom.Value.MilliValue() > max.Custom.Value.MilliValue() {
				max = value
			}
		}

	}
	max.Custom.Value = *resource.NewMilliQuantity(int64(float64(max.Custom.Value.MilliValue())*c.weight), resource.DecimalSI)
	return []CollectedMetric{max}, nil
}

// Interval returns the interval at which the collector should run.
func (c *WeightedMaxCollector) Interval() time.Duration {
	return c.interval
}
