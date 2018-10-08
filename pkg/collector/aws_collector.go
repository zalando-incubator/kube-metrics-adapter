package collector

import (
	"fmt"
	"strconv"
	"time"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/sqs"
	"github.com/aws/aws-sdk-go/service/sqs/sqsiface"
	autoscalingv2beta1 "k8s.io/api/autoscaling/v2beta1"
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
	sessions map[string]*session.Session
}

func NewAWSCollectorPlugin(sessions map[string]*session.Session) *AWSCollectorPlugin {
	return &AWSCollectorPlugin{
		sessions: sessions,
	}
}

// NewCollector initializes a new skipper collector from the specified HPA.
func (c *AWSCollectorPlugin) NewCollector(hpa *autoscalingv2beta1.HorizontalPodAutoscaler, config *MetricConfig, interval time.Duration) (Collector, error) {
	switch config.Name {
	case AWSSQSQueueLengthMetric:
		return NewAWSSQSCollector(c.sessions, config, interval)
	}

	return nil, fmt.Errorf("metric '%s' not supported", config.Name)
}

type AWSSQSCollector struct {
	sqs        sqsiface.SQSAPI
	interval   time.Duration
	region     string
	queueURL   string
	queueName  string
	labels     map[string]string
	metricName string
	metricType autoscalingv2beta1.MetricSourceType
}

func NewAWSSQSCollector(sessions map[string]*session.Session, config *MetricConfig, interval time.Duration) (*AWSSQSCollector, error) {

	name, ok := config.Labels[sqsQueueNameLabelKey]
	if !ok {
		return nil, fmt.Errorf("sqs queue name not specified on metric")
	}
	region, ok := config.Labels[sqsQueueRegionLabelKey]
	if !ok {
		return nil, fmt.Errorf("sqs queue region is not specified on metric")
	}

	session, ok := sessions[region]
	if !ok {
		return nil, fmt.Errorf("the metric region: %s is not configured", region)
	}

	service := sqs.New(session)
	params := &sqs.GetQueueUrlInput{
		QueueName: aws.String(name),
	}

	resp, err := service.GetQueueUrl(params)
	if err != nil {
		return nil, fmt.Errorf("failed to get queue URL for queue '%s': %v", name, err)
	}

	return &AWSSQSCollector{
		sqs:        service,
		interval:   interval,
		queueURL:   aws.StringValue(resp.QueueUrl),
		queueName:  name,
		metricName: config.Name,
		metricType: config.Type,
		labels:     config.Labels,
	}, nil
}

func (c *AWSSQSCollector) GetMetrics() ([]CollectedMetric, error) {
	params := &sqs.GetQueueAttributesInput{
		QueueUrl:       aws.String(c.queueURL),
		AttributeNames: aws.StringSlice([]string{sqs.QueueAttributeNameApproximateNumberOfMessages}),
	}

	resp, err := c.sqs.GetQueueAttributes(params)
	if err != nil {
		return nil, err
	}

	if v, ok := resp.Attributes[sqs.QueueAttributeNameApproximateNumberOfMessages]; ok {
		i, err := strconv.Atoi(aws.StringValue(v))
		if err != nil {
			return nil, err
		}

		metricValue := CollectedMetric{
			Type: c.metricType,
			External: external_metrics.ExternalMetricValue{
				MetricName:   c.metricName,
				MetricLabels: c.labels,
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
