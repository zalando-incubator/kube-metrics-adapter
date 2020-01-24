package collector

import (
	"context"
	"fmt"
	"time"

	"github.com/influxdata/influxdb-client-go"
	"k8s.io/api/autoscaling/v2beta2"
	autoscalingv2 "k8s.io/api/autoscaling/v2beta2"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/metrics/pkg/apis/external_metrics"
)

const (
	InfluxDBMetricName        = "flux-query"
	influxDBAddressKey        = "address"
	influxDBTokenKey          = "token"
	influxDBOrgIDKey          = "org-id"
	influxDBQueryNameLabelKey = "query-name"
)

type InfluxDBCollectorPlugin struct {
	kubeClient kubernetes.Interface
	address    string
	token      string
	orgID      string
}

func NewInfluxDBCollectorPlugin(client kubernetes.Interface, address, token, orgID string) (*InfluxDBCollectorPlugin, error) {
	return &InfluxDBCollectorPlugin{
		kubeClient: client,
		address:    address,
		token:      token,
		orgID:      orgID,
	}, nil
}

func (p *InfluxDBCollectorPlugin) NewCollector(hpa *v2beta2.HorizontalPodAutoscaler, config *MetricConfig, interval time.Duration) (Collector, error) {
	return NewInfluxDBCollector(p.address, p.orgID, p.token, config, interval)
}

type InfluxDBCollector struct {
	address string
	token   string
	orgID   string

	influxDBClient *influxdb.Client
	interval       time.Duration
	metric         autoscalingv2.MetricIdentifier
	metricType     autoscalingv2.MetricSourceType
	query          string
}

func NewInfluxDBCollector(address string, token string, orgID string, config *MetricConfig, interval time.Duration) (*InfluxDBCollector, error) {
	collector := &InfluxDBCollector{
		interval:   interval,
		metric:     config.Metric,
		metricType: config.Type,
	}
	switch configType := config.Type; configType {
	case autoscalingv2.ObjectMetricSourceType:
		return nil, fmt.Errorf("InfluxDB does not support object, but only external custom metrics")
	case autoscalingv2.ExternalMetricSourceType:
		// `metricSelector` is flattened into the MetricConfig.Config.
		queryName, ok := config.Config[influxDBQueryNameLabelKey]
		if !ok {
			return nil, fmt.Errorf("selector for Flux query is not specified, "+
				"please add metricSelector.matchLabels.%s: <...> to .yml description", influxDBQueryNameLabelKey)
		}
		if query, ok := config.Config[queryName]; ok {
			// TODO(affo): validate the query once this is done:
			//  https://github.com/influxdata/influxdb-client-go/issues/73.
			collector.query = query
		} else {
			return nil, fmt.Errorf("no Flux query defined for metric \"%s\"", config.Metric.Name)
		}
	default:
		return nil, fmt.Errorf("unknown metric type: %v", configType)
	}
	// Use custom InfluxDB config if defined in HPA annotation.
	if v, ok := config.Config[influxDBAddressKey]; ok {
		address = v
	}
	if v, ok := config.Config[influxDBTokenKey]; ok {
		token = v
	}
	if v, ok := config.Config[influxDBOrgIDKey]; ok {
		orgID = v
	}
	influxDbClient, err := influxdb.New(address, token)
	if err != nil {
		return nil, err
	}
	collector.address = address
	collector.token = token
	collector.orgID = orgID
	collector.influxDBClient = influxDbClient
	return collector, nil
}

// queryResult is for unmarshaling the result from InfluxDB.
// The FluxQuery should make it so that the resulting table contains the column "metricvalue".
type queryResult struct {
	MetricValue float64
}

// getValue returns the first result gathered from an InfluxDB instance.
func (c *InfluxDBCollector) getValue() (resource.Quantity, error) {
	res, err := c.influxDBClient.QueryCSV(context.Background(), c.query, c.orgID)
	if err != nil {
		return resource.Quantity{}, err
	}
	defer res.Close()
	// Keeping just the first result.
	if res.Next() {
		qr := queryResult{}
		if err := res.Unmarshal(&qr); err != nil {
			return resource.Quantity{}, fmt.Errorf("error in unmarshaling query result: %v", err)
		}
		return *resource.NewMilliQuantity(int64(qr.MetricValue*1000), resource.DecimalSI), nil
	}
	if err := res.Err; err != nil {
		return resource.Quantity{}, fmt.Errorf("error in query result: %v", err)
	}
	return resource.Quantity{}, fmt.Errorf("empty result returned")
}

func (c *InfluxDBCollector) GetMetrics() ([]CollectedMetric, error) {
	v, err := c.getValue()
	if err != nil {
		return nil, err
	}
	cm := CollectedMetric{
		Type: c.metricType,
		External: external_metrics.ExternalMetricValue{
			MetricName:   c.metric.Name,
			MetricLabels: c.metric.Selector.MatchLabels,
			Timestamp: metav1.Time{
				Time: time.Now().UTC(),
			},
			Value: v,
		},
	}
	return []CollectedMetric{cm}, nil
}

func (c *InfluxDBCollector) Interval() time.Duration {
	return c.interval
}
