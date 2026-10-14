package collector

import (
	"context"
	"fmt"
	"math"
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
	sqsIncludeInFlightKey   = "include-in-flight"
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
func (c *AWSCollectorPlugin) NewCollector(ctx context.Context, hpa *autoscalingv2.HorizontalPodAutoscaler, config *MetricConfig, interval time.Duration) (Collector, error) {
	return NewAWSSQSCollector(ctx, c.configs, hpa, config, interval)
}

type sqsiface interface {
	GetQueueAttributes(ctx context.Context, params *sqs.GetQueueAttributesInput, optFns ...func(*sqs.Options)) (*sqs.GetQueueAttributesOutput, error)
}

type AWSSQSCollector struct {
	sqs             sqsiface
	interval        time.Duration
	queueURL        string
	queueName       string
	namespace       string
	metric          autoscalingv2.MetricIdentifier
	metricType      autoscalingv2.MetricSourceType
	includeInFlight bool
}

func NewAWSSQSCollector(ctx context.Context, configs map[string]aws.Config, hpa *autoscalingv2.HorizontalPodAutoscaler, config *MetricConfig, interval time.Duration) (*AWSSQSCollector, error) {
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

	var includeInFlight bool
	if value, ok := config.Config[sqsIncludeInFlightKey]; ok {
		var err error
		includeInFlight, err = strconv.ParseBool(value)
		if err != nil {
			return nil, fmt.Errorf("invalid %s value %q: %w", sqsIncludeInFlightKey, value, err)
		}
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
		sqs:             service,
		interval:        interval,
		queueURL:        aws.ToString(resp.QueueUrl),
		queueName:       name,
		namespace:       hpa.Namespace,
		metric:          config.Metric,
		metricType:      config.Type,
		includeInFlight: includeInFlight,
	}, nil
}

func (c *AWSSQSCollector) GetMetrics(ctx context.Context) ([]CollectedMetric, error) {
	params := &sqs.GetQueueAttributesInput{
		QueueUrl:       aws.String(c.queueURL),
		AttributeNames: []types.QueueAttributeName{types.QueueAttributeNameApproximateNumberOfMessages},
	}

	if c.includeInFlight {
		params.AttributeNames = append(params.AttributeNames, types.QueueAttributeNameApproximateNumberOfMessagesNotVisible)
	}

	resp, err := c.sqs.GetQueueAttributes(ctx, params)
	if err != nil {
		return nil, err
	}

	var total int64
	for _, attribute := range params.AttributeNames {
		value, ok := resp.Attributes[string(attribute)]
		if !ok {
			return nil, fmt.Errorf("missing %s for queue %q", attribute, c.queueName)
		}
		count, err := strconv.ParseInt(value, 10, 64)
		if err != nil {
			return nil, fmt.Errorf("invalid %s for queue %q: %w", attribute, c.queueName, err)
		}
		if count < 0 {
			return nil, fmt.Errorf("negative %s for queue %q", attribute, c.queueName)
		}
		if count > math.MaxInt64-total {
			return nil, fmt.Errorf("queue length overflow for queue %q", c.queueName)
		}
		total += count
	}

	metricValue := CollectedMetric{
		Namespace: c.namespace,
		Type:      c.metricType,
		External: external_metrics.ExternalMetricValue{
			MetricName:   c.metric.Name,
			MetricLabels: c.metric.Selector.MatchLabels,
			Timestamp:    metav1.Time{Time: time.Now().UTC()},
			Value:        *resource.NewQuantity(total, resource.DecimalSI),
		},
	}

	return []CollectedMetric{metricValue}, nil
}

// Interval returns the interval at which the collector should run.
func (c *AWSSQSCollector) Interval() time.Duration {
	return c.interval
}
