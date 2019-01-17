package collector

import (
	"fmt"
	"k8s.io/apimachinery/pkg/api/resource"
	"time"
)

// MaxWeightedCollector is a simple aggregator collector that returns the maximum value
// of metrics from all collectors.
type MaxWeightedCollector struct {
	collectors []Collector
	interval   time.Duration
	weight     float64
}

// NewMaxCollector initializes a new MacCollector.
func NewMaxWeightedCollector(interval time.Duration, weight float64, collectors ...Collector) *MaxWeightedCollector {
	return &MaxWeightedCollector{
		collectors: collectors,
		interval:   interval,
		weight:     weight,
	}
}

// GetMetrics gets metrics from all collectors and return the higest value.
func (c *MaxWeightedCollector) GetMetrics() ([]CollectedMetric, error) {
	collectedMetrics := make([]CollectedMetric, 0)
	for _, collector := range c.collectors {
		values, err := collector.GetMetrics()
		if err != nil {
			return nil, err
		}
		for _, v := range values {
			collectedMetrics = append(collectedMetrics, v)
		}
	}
	if len(collectedMetrics) == 0 {
		return nil, fmt.Errorf("no metrics collected, cannot determine max")
	}
	max := collectedMetrics[0]
	for _, value := range collectedMetrics {
		if value.Custom.Value.MilliValue() > max.Custom.Value.MilliValue() {
			max = value
		}
	}
	max.Custom.Value = *resource.NewMilliQuantity(int64(c.weight*float64(max.Custom.Value.MilliValue())), resource.DecimalSI)
	return []CollectedMetric{max}, nil
}

// Interval returns the interval at which the collector should run.
func (c *MaxWeightedCollector) Interval() time.Duration {
	return c.interval
}
