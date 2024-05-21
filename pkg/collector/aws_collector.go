package collector

import (
	"context"
	"fmt"
	"strconv"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/sqs"
	"github.com/aws/aws-sdk-go-v2/service/sqs/types"
	autoscalingv2 "k8s.io/api/autoscaling/v2"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/metrics/pkg/apis/external_metrics"
)

const (
	AWSSQSQueueLengthMetric = "sqs-queue-length"
	sqsQueueNameLabelKey    = "queue-name"
	sqsQueueRegionLabelKey  = "region"
)

type AWSCollectorPlugin struct {
	configs map[string]aws.Config
}

func NewAWSCollectorPlugin(configs map[string]aws.Config) *AWSCollectorPlugin {
	return &AWSCollectorPlugin{
		configs: configs,
	}
}

// NewCollector initializes a new skipper collector from the specified HPA.
func (c *AWSCollectorPlugin) NewCollector(hpa *autoscalingv2.HorizontalPodAutoscaler, config *MetricConfig, interval time.Duration) (Collector, error) {
	return NewAWSSQSCollector(c.configs, hpa, config, interval)
}

type sqsiface interface {
	GetQueueAttributes(ctx context.Context, params *sqs.GetQueueAttributesInput, optFns ...func(*sqs.Options)) (*sqs.GetQueueAttributesOutput, error)
}

type AWSSQSCollector struct {
	sqs        sqsiface
	interval   time.Duration
	queueURL   string
	queueName  string
	namespace  string
	metric     autoscalingv2.MetricIdentifier
	metricType autoscalingv2.MetricSourceType
}

func NewAWSSQSCollector(configs map[string]aws.Config, hpa *autoscalingv2.HorizontalPodAutoscaler, config *MetricConfig, interval time.Duration) (*AWSSQSCollector, error) {
	if config.Metric.Selector == nil {
		return nil, fmt.Errorf("selector for queue is not specified")
	}

	name, ok := config.Config[sqsQueueNameLabelKey]
	if !ok {
		return nil, fmt.Errorf("sqs queue name not specified on metric")
	}
	region, ok := config.Config[sqsQueueRegionLabelKey]
	if !ok {
		return nil, fmt.Errorf("sqs queue region is not specified on metric")
	}

	cfg, ok := configs[region]
	if !ok {
		return nil, fmt.Errorf("the metric region: %s is not configured", region)
	}

	service := sqs.NewFromConfig(cfg)
	params := &sqs.GetQueueUrlInput{
		QueueName: aws.String(name),
	}

	resp, err := service.GetQueueUrl(context.TODO(), params)
	if err != nil {
		return nil, fmt.Errorf("failed to get queue URL for queue '%s': %v", name, err)
	}

	return &AWSSQSCollector{
		sqs:        service,
		interval:   interval,
		queueURL:   aws.ToString(resp.QueueUrl),
		queueName:  name,
		namespace:  hpa.Namespace,
		metric:     config.Metric,
		metricType: config.Type,
	}, nil
}

func (c *AWSSQSCollector) GetMetrics() ([]CollectedMetric, error) {
	params := &sqs.GetQueueAttributesInput{
		QueueUrl:       aws.String(c.queueURL),
		AttributeNames: []types.QueueAttributeName{types.QueueAttributeNameApproximateNumberOfMessages},
	}

	resp, err := c.sqs.GetQueueAttributes(context.TODO(), params)
	if err != nil {
		return nil, err
	}

	if v, ok := resp.Attributes[string(types.QueueAttributeNameApproximateNumberOfMessages)]; ok {
		i, err := strconv.Atoi(v)
		if err != nil {
			return nil, err
		}

		metricValue := CollectedMetric{
			Namespace: c.namespace,
			Type:      c.metricType,
			External: external_metrics.ExternalMetricValue{
				MetricName:   c.metric.Name,
				MetricLabels: c.metric.Selector.MatchLabels,
				Timestamp:    metav1.Time{Time: time.Now().UTC()},
				Value:        *resource.NewQuantity(int64(i), resource.DecimalSI),
			},
		}

		return []CollectedMetric{metricValue}, nil
	}

	return nil, fmt.Errorf("failed to get queue length for '%s'", c.queueName)
}

// Interval returns the interval at which the collector should run.
func (c *AWSSQSCollector) Interval() time.Duration {
	return c.interval
}
