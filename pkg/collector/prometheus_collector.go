package collector

import (
	"context"
	"fmt"
	"net/http"
	"time"

	"github.com/prometheus/client_golang/api"
	promv1 "github.com/prometheus/client_golang/api/prometheus/v1"
	"github.com/prometheus/common/model"
	autoscalingv2 "k8s.io/api/autoscaling/v2beta2"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/metrics/pkg/apis/custom_metrics"
)

type PrometheusCollectorPlugin struct {
	promAPI promv1.API
	client  kubernetes.Interface
}

func NewPrometheusCollectorPlugin(client kubernetes.Interface, prometheusServer string) (*PrometheusCollectorPlugin, error) {
	cfg := api.Config{
		Address:      prometheusServer,
		RoundTripper: &http.Transport{},
	}

	promClient, err := api.NewClient(cfg)
	if err != nil {
		return nil, err
	}

	return &PrometheusCollectorPlugin{
		client:  client,
		promAPI: promv1.NewAPI(promClient),
	}, nil
}

func (p *PrometheusCollectorPlugin) NewCollector(hpa *autoscalingv2.HorizontalPodAutoscaler, config *MetricConfig, interval time.Duration) (Collector, error) {
	return NewPrometheusCollector(p.client, p.promAPI, hpa, config, interval)
}

type PrometheusCollector struct {
	client          kubernetes.Interface
	promAPI         promv1.API
	query           string
	metric          autoscalingv2.MetricIdentifier
	metricType      autoscalingv2.MetricSourceType
	objectReference custom_metrics.ObjectReference
	interval        time.Duration
	perReplica      bool
	hpa             *autoscalingv2.HorizontalPodAutoscaler
}

func NewPrometheusCollector(client kubernetes.Interface, promAPI promv1.API, hpa *autoscalingv2.HorizontalPodAutoscaler, config *MetricConfig, interval time.Duration) (*PrometheusCollector, error) {
	c := &PrometheusCollector{
		client:          client,
		objectReference: config.ObjectReference,
		metric:          config.Metric,
		metricType:      config.Type,
		interval:        interval,
		promAPI:         promAPI,
		perReplica:      config.PerReplica,
		hpa:             hpa,
	}

	if v, ok := config.Config["query"]; ok {
		// TODO: validate query
		c.query = v
	} else {
		return nil, fmt.Errorf("no prometheus query defined")
	}

	return c, nil
}

func (c *PrometheusCollector) GetMetrics() ([]CollectedMetric, error) {
	// TODO: use real context
	value, err := c.promAPI.Query(context.Background(), c.query, time.Now().UTC())
	if err != nil {
		return nil, err
	}

	var sampleValue model.SampleValue
	switch value.Type() {
	case model.ValVector:
		samples := value.(model.Vector)
		if len(samples) == 0 {
			return nil, fmt.Errorf("query '%s' returned no samples", c.query)
		}

		sampleValue = samples[0].Value
	case model.ValScalar:
		scalar := value.(*model.Scalar)
		sampleValue = scalar.Value
	}

	if sampleValue.String() == "NaN" {
		return nil, fmt.Errorf("query '%s' returned no samples: %s", c.query, sampleValue.String())
	}

	if c.perReplica {
		// get current replicas for the targeted scale object. This is used to
		// calculate an average metric instead of total.
		// targetAverageValue will be available in Kubernetes v1.12
		// https://github.com/kubernetes/kubernetes/pull/64097
		replicas, err := targetRefReplicas(c.client, c.hpa)
		if err != nil {
			return nil, err
		}
		sampleValue = model.SampleValue(float64(sampleValue) / float64(replicas))
	}

	metricValue := CollectedMetric{
		Type: c.metricType,
		Custom: custom_metrics.MetricValue{
			DescribedObject: c.objectReference,
			Metric:          custom_metrics.MetricIdentifier{Name: c.metric.Name, Selector: c.metric.Selector},
			Timestamp:       metav1.Time{Time: time.Now().UTC()},
			Value:           *resource.NewMilliQuantity(int64(sampleValue*1000), resource.DecimalSI),
		},
	}

	return []CollectedMetric{metricValue}, nil
}

func (c *PrometheusCollector) Interval() time.Duration {
	return c.interval
}
