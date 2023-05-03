package collector

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"time"

	autoscalingv2 "k8s.io/api/autoscaling/v2beta2"
)

const (
	HostnameMetricType = "requests-per-second"
	HostnameRPSQuery   = `scalar(sum(rate(%s{host=~"%s"}[1m])) * %.4f)`
)

type HostnameCollectorPlugin struct {
	metricName string
	promPlugin CollectorPlugin
}

type HostnameCollector struct {
	interval      time.Duration
	promCollector Collector
}

func NewHostnameCollectorPlugin(
	promPlugin CollectorPlugin,
	metricName string,
) (*HostnameCollectorPlugin, error) {
	if metricName == "" {
		return nil, fmt.Errorf("failed to initialize hostname collector plugin, metric name was not defined")
	}

	return &HostnameCollectorPlugin{
		metricName: metricName,
		promPlugin: promPlugin,
	}, nil
}

// NewCollector initializes a new skipper collector from the specified HPA.
func (p *HostnameCollectorPlugin) NewCollector(
	hpa *autoscalingv2.HorizontalPodAutoscaler,
	config *MetricConfig,
	interval time.Duration,
) (Collector, error) {
	if config == nil {
		return nil, fmt.Errorf("Metric config not present, it is not possible to initialize the collector.")
	}
	// Need to copy config and add a promQL query in order to get
	// RPS data from a specific hostname from prometheus. The idea
	// of the copy is to not modify the original config struct.
	confCopy := *config
	//hostnames := config.Config["hostnames"]
	//weights := config.Config["weights"]

	if _, ok := config.Config["hostnames"]; !ok {
		return nil, fmt.Errorf("hostname is not specified, unable to create collector")
	}
	hostnames := strings.Split(config.Config["hostnames"], ",")
	for _, h := range hostnames {
		if ok, err := regexp.MatchString("^[a-zA-Z0-9.-]+$", h); !ok || err != nil {
			return nil, fmt.Errorf(
				"Invalid hostname format, unable to create collector: %s",
				h,
			)
		}
	}

	weight := 1.0
	if w, ok := config.Config["weight"]; ok {
		num, err := strconv.ParseFloat(w, 64)
		if err != nil {
			return nil, fmt.Errorf("Could not parse weight annotation, unable to create collector: %s", w)
		}
		weight = num / 100.0
	}

	confCopy.Config = map[string]string{
		"query": fmt.Sprintf(
			HostnameRPSQuery,
			p.metricName,
			strings.Replace(strings.Join(hostnames, "|"), ".", "_", -1),
			weight,
		),
	}

	c, err := p.promPlugin.NewCollector(hpa, &confCopy, interval)
	if err != nil {
		return nil, err
	}

	return &HostnameCollector{
		interval:      interval,
		promCollector: c,
	}, nil
}

// GetMetrics gets hostname metrics from Prometheus
func (c *HostnameCollector) GetMetrics() ([]CollectedMetric, error) {
	v, err := c.promCollector.GetMetrics()
	if err != nil {
		return nil, err
	}

	if len(v) != 1 {
		return nil, fmt.Errorf("expected to only get one metric value, got %d", len(v))
	}
	return v, nil
}

// Interval returns the interval at which the collector should run.
func (c *HostnameCollector) Interval() time.Duration {
	return c.interval
}
