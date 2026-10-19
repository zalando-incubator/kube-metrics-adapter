package collector

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/stretchr/testify/require"
	autoscalingv2 "k8s.io/api/autoscaling/v2"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// fakeSQSAPI implements the AWS HTTP boundary while exercising the real SDK.
type fakeSQSAPI struct {
	attributes          map[string]string
	attributesError     error
	requestedAttributes []string
	requests            int
}

func (a *fakeSQSAPI) Do(request *http.Request) (*http.Response, error) {
	a.requests++
	var result any
	switch request.Header.Get("X-Amz-Target") {
	case "AmazonSQS.GetQueueUrl":
		result = map[string]string{"QueueUrl": "https://sqs.example.test/queue"}
	case "AmazonSQS.GetQueueAttributes":
		if a.attributesError != nil {
			return nil, a.attributesError
		}
		var input struct {
			AttributeNames []string
		}
		if err := json.NewDecoder(request.Body).Decode(&input); err != nil {
			return nil, err
		}
		a.requestedAttributes = input.AttributeNames
		attributes := map[string]string{}
		for _, name := range input.AttributeNames {
			if value, ok := a.attributes[name]; ok {
				attributes[name] = value
			}
		}
		result = map[string]any{"Attributes": attributes}
	default:
		return nil, fmt.Errorf("unexpected AWS operation: %s", request.Header.Get("X-Amz-Target"))
	}

	body, err := json.Marshal(result)
	if err != nil {
		return nil, err
	}
	return &http.Response{
		StatusCode: http.StatusOK,
		Header:     http.Header{"Content-Type": {"application/x-amz-json-1.0"}},
		Body:       io.NopCloser(strings.NewReader(string(body))),
		Request:    request,
	}, nil
}

func newTestAWSSQSCollector(t *testing.T, api *fakeSQSAPI, extraLabels map[string]string) (Collector, error) {
	t.Helper()
	metricLabels := map[string]string{
		"type":       "sqs-queue-length",
		"queue-name": "worker-queue",
		"region":     "eu-central-1",
	}
	for key, value := range extraLabels {
		metricLabels[key] = value
	}
	hpa := &autoscalingv2.HorizontalPodAutoscaler{
		ObjectMeta: metav1.ObjectMeta{Namespace: "workers"},
		Spec: autoscalingv2.HorizontalPodAutoscalerSpec{
			Metrics: []autoscalingv2.MetricSpec{{
				Type: autoscalingv2.ExternalMetricSourceType,
				External: &autoscalingv2.ExternalMetricSource{
					Metric: autoscalingv2.MetricIdentifier{
						Name:     "worker-sqs-queue-length",
						Selector: &metav1.LabelSelector{MatchLabels: metricLabels},
					},
				},
			}},
		},
	}
	configs, err := ParseHPAMetrics(hpa)
	require.NoError(t, err)
	require.Len(t, configs, 1)

	factory := NewCollectorFactory()
	factory.RegisterExternalCollector([]string{AWSSQSQueueLengthMetric}, NewAWSCollectorPlugin(map[string]aws.Config{
		"eu-central-1": {
			Region:           "eu-central-1",
			HTTPClient:       api,
			RetryMaxAttempts: 1,
			Credentials: aws.CredentialsProviderFunc(func(context.Context) (aws.Credentials, error) {
				return aws.Credentials{AccessKeyID: "test", SecretAccessKey: "test"}, nil
			}),
		},
	}))
	return factory.NewCollector(context.Background(), hpa, configs[0], 30*time.Second)
}

func TestAWSSQSCollectorIncludesInFlightMessages(t *testing.T) {
	api := &fakeSQSAPI{attributes: map[string]string{
		"ApproximateNumberOfMessages":           "3",
		"ApproximateNumberOfMessagesNotVisible": "7",
	}}
	collector, err := newTestAWSSQSCollector(t, api, map[string]string{"include-in-flight": "true"})
	require.NoError(t, err)

	started := time.Now()
	metrics, err := collector.GetMetrics(context.Background())
	require.NoError(t, err)
	require.Len(t, metrics, 1)
	require.EqualValues(t, 10, metrics[0].External.Value.Value())
	require.Equal(t, "workers", metrics[0].Namespace)
	require.Equal(t, autoscalingv2.ExternalMetricSourceType, metrics[0].Type)
	require.Equal(t, "worker-sqs-queue-length", metrics[0].External.MetricName)
	require.Equal(t, map[string]string{
		"type":              "sqs-queue-length",
		"queue-name":        "worker-queue",
		"region":            "eu-central-1",
		"include-in-flight": "true",
	}, metrics[0].External.MetricLabels)
	require.WithinRange(t, metrics[0].External.Timestamp.Time, started, time.Now())
	require.Equal(t, 30*time.Second, collector.Interval())
	require.ElementsMatch(t, []string{
		"ApproximateNumberOfMessages",
		"ApproximateNumberOfMessagesNotVisible",
	}, api.requestedAttributes)
	require.Equal(t, 2, api.requests, "one queue URL lookup and one attribute request")
}

func TestAWSSQSCollectorRejectsInvalidInFlightOptionBeforeCallingAWS(t *testing.T) {
	for _, value := range []string{"", "yes", "invalid"} {
		t.Run(value, func(t *testing.T) {
			api := &fakeSQSAPI{}
			_, err := newTestAWSSQSCollector(t, api, map[string]string{"include-in-flight": value})
			require.ErrorContains(t, err, "include-in-flight")
			require.Zero(t, api.requests)
		})
	}
}

func TestAWSSQSCollectorKeepsVisibleOnlyCountingByDefault(t *testing.T) {
	for _, tc := range []struct {
		name     string
		labels   map[string]string
		inFlight string
	}{
		{name: "option omitted"},
		{name: "explicit false", labels: map[string]string{"include-in-flight": "false"}, inFlight: "7"},
		{name: "invalid unrequested count ignored", labels: map[string]string{"include-in-flight": "false"}, inFlight: "invalid"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			api := &fakeSQSAPI{attributes: map[string]string{"ApproximateNumberOfMessages": "3"}}
			if tc.inFlight != "" {
				api.attributes["ApproximateNumberOfMessagesNotVisible"] = tc.inFlight
			}
			collector, err := newTestAWSSQSCollector(t, api, tc.labels)
			require.NoError(t, err)

			metrics, err := collector.GetMetrics(context.Background())
			require.NoError(t, err)
			require.Len(t, metrics, 1)
			require.EqualValues(t, 3, metrics[0].External.Value.Value())
			require.Equal(t, []string{"ApproximateNumberOfMessages"}, api.requestedAttributes)
		})
	}
}

func TestAWSSQSCollectorCountsInFlightMessagesWhenQueueHasNoVisibleMessages(t *testing.T) {
	api := &fakeSQSAPI{attributes: map[string]string{
		"ApproximateNumberOfMessages":           "0",
		"ApproximateNumberOfMessagesNotVisible": "20",
	}}
	collector, err := newTestAWSSQSCollector(t, api, map[string]string{"include-in-flight": "true"})
	require.NoError(t, err)

	metrics, err := collector.GetMetrics(context.Background())
	require.NoError(t, err)
	require.Len(t, metrics, 1)
	require.EqualValues(t, 20, metrics[0].External.Value.Value())
}

func TestAWSSQSCollectorReturnsZeroWhenBothCountsAreZero(t *testing.T) {
	api := &fakeSQSAPI{attributes: map[string]string{
		"ApproximateNumberOfMessages":           "0",
		"ApproximateNumberOfMessagesNotVisible": "0",
	}}
	collector, err := newTestAWSSQSCollector(t, api, map[string]string{"include-in-flight": "true"})
	require.NoError(t, err)

	metrics, err := collector.GetMetrics(context.Background())
	require.NoError(t, err)
	require.Len(t, metrics, 1)
	require.Zero(t, metrics[0].External.Value.Value())
}

func TestAWSSQSCollectorRejectsMissingOrInvalidRequiredCounts(t *testing.T) {
	for _, attribute := range []string{"ApproximateNumberOfMessages", "ApproximateNumberOfMessagesNotVisible"} {
		for _, value := range []string{"", "invalid", "-1", "9223372036854775808"} {
			t.Run(attribute+"/"+value, func(t *testing.T) {
				api := &fakeSQSAPI{attributes: map[string]string{
					"ApproximateNumberOfMessages":           "3",
					"ApproximateNumberOfMessagesNotVisible": "7",
				}}
				if value == "" {
					delete(api.attributes, attribute)
				} else {
					api.attributes[attribute] = value
				}
				collector, err := newTestAWSSQSCollector(t, api, map[string]string{"include-in-flight": "true"})
				require.NoError(t, err)

				metrics, err := collector.GetMetrics(context.Background())
				require.ErrorContains(t, err, attribute)
				require.Empty(t, metrics)
			})
		}
	}
}

func TestAWSSQSCollectorPropagatesAWSErrorsWithoutReturningAMetric(t *testing.T) {
	awsError := errors.New("queue unavailable")
	api := &fakeSQSAPI{attributesError: awsError}
	collector, err := newTestAWSSQSCollector(t, api, map[string]string{"include-in-flight": "true"})
	require.NoError(t, err)

	metrics, err := collector.GetMetrics(context.Background())
	require.ErrorIs(t, err, awsError)
	require.Empty(t, metrics)
}

func TestAWSSQSCollectorRejectsOverflowingTotal(t *testing.T) {
	api := &fakeSQSAPI{attributes: map[string]string{
		"ApproximateNumberOfMessages":           "9223372036854775807",
		"ApproximateNumberOfMessagesNotVisible": "1",
	}}
	collector, err := newTestAWSSQSCollector(t, api, map[string]string{"include-in-flight": "true"})
	require.NoError(t, err)

	metrics, err := collector.GetMetrics(context.Background())
	require.ErrorContains(t, err, "overflow")
	require.Empty(t, metrics)
}

func TestAWSSQSCollectorRejectsNegativeMessageCounts(t *testing.T) {
	api := &fakeSQSAPI{attributes: map[string]string{
		"ApproximateNumberOfMessages":           "3",
		"ApproximateNumberOfMessagesNotVisible": "-1",
	}}
	collector, err := newTestAWSSQSCollector(t, api, map[string]string{"include-in-flight": "true"})
	require.NoError(t, err)

	metrics, err := collector.GetMetrics(context.Background())
	require.ErrorContains(t, err, "ApproximateNumberOfMessagesNotVisible")
	require.Empty(t, metrics)
}
