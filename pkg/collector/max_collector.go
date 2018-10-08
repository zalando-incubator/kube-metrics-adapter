package collector

import "time"

// MaxCollector is a simple aggregator collector that returns the maximum value
// of metrics from all collectors.
type MaxCollector struct {
	collectors []Collector
	interval   time.Duration
}

// NewMaxCollector initializes a new MacCollector.
func NewMaxCollector(interval time.Duration, collectors ...Collector) *MaxCollector {
	return &MaxCollector{
		collectors: collectors,
		interval:   interval,
	}
}

// GetMetrics gets metrics from all collectors and return the higest value.
func (c *MaxCollector) GetMetrics() ([]CollectedMetric, error) {
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
	return []CollectedMetric{max}, nil
}

// Interval returns the interval at which the collector should run.
func (c *MaxCollector) Interval() time.Duration {
	return c.interval
}
