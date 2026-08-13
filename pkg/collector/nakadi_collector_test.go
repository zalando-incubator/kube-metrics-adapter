package collector

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	autoscalingv2 "k8s.io/api/autoscaling/v2"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func TestNewNakadiCollectorTrimsEventTypes(t *testing.T) {
	hpa := &autoscalingv2.HorizontalPodAutoscaler{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: "default",
		},
	}

	config := &MetricConfig{
		MetricTypeName: MetricTypeName{
			Type: autoscalingv2.ExternalMetricSourceType,
			Metric: autoscalingv2.MetricIdentifier{
				Name: "nakadi-metric",
				Selector: &metav1.LabelSelector{
					MatchLabels: map[string]string{"type": NakadiMetricType},
				},
			},
		},
		Config: map[string]string{
			nakadiMetricTypeKey:        nakadiMetricTypeUnconsumedEvents,
			nakadiOwningApplicationKey: "example-app",
			nakadiEventTypesKey:        "event-a, event-b,   ,event-c  ",
			nakadiConsumerGroupKey:     "example-group",
		},
	}

	collector, err := NewNakadiCollector(context.Background(), nil, hpa, config, time.Second)
	require.NoError(t, err)
	require.Equal(t, []string{"event-a", "event-b", "event-c"}, collector.subscriptionFilter.EventTypes)
}

func TestNewNakadiCollectorRejectsBlankEventTypesAfterTrim(t *testing.T) {
	hpa := &autoscalingv2.HorizontalPodAutoscaler{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: "default",
		},
	}

	config := &MetricConfig{
		MetricTypeName: MetricTypeName{
			Type: autoscalingv2.ExternalMetricSourceType,
			Metric: autoscalingv2.MetricIdentifier{
				Name: "nakadi-metric",
				Selector: &metav1.LabelSelector{
					MatchLabels: map[string]string{"type": NakadiMetricType},
				},
			},
		},
		Config: map[string]string{
			nakadiMetricTypeKey:        nakadiMetricTypeUnconsumedEvents,
			nakadiOwningApplicationKey: "example-app",
			nakadiEventTypesKey:        "  , ,  ",
			nakadiConsumerGroupKey:     "example-group",
		},
	}

	_, err := NewNakadiCollector(context.Background(), nil, hpa, config, time.Second)
	require.EqualError(t, err, "either subscription-id or all of [owning-application, event-types, consumer-group] must be specified on the metric")
}
