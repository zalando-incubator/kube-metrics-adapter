package collector

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	autoscalingv2 "k8s.io/api/autoscaling/v2"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

type mockCollectorPlugin struct {
	Name string
}

func (c *mockCollectorPlugin) NewCollector(_ context.Context, hpa *autoscalingv2.HorizontalPodAutoscaler, config *MetricConfig, interval time.Duration) (Collector, error) {
	return &mockCollector{Name: c.Name}, nil
}

type mockCollector struct {
	Name string
}

func (c *mockCollector) GetMetrics(_ context.Context) ([]CollectedMetric, error) {
	return nil, nil
}

func (c *mockCollector) Interval() time.Duration {
	return 0
}

func TestNewCollector(t *testing.T) {
	for _, tc := range []struct {
		msg               string
		hpa               *autoscalingv2.HorizontalPodAutoscaler
		expectedCollector string
	}{
		{
			msg: "should get create collector type from legacy metric name",
			hpa: &autoscalingv2.HorizontalPodAutoscaler{
				ObjectMeta: metav1.ObjectMeta{
					Annotations: map[string]string{},
				},
				Spec: autoscalingv2.HorizontalPodAutoscalerSpec{
					Metrics: []autoscalingv2.MetricSpec{
						{
							Type: autoscalingv2.ExternalMetricSourceType,
							External: &autoscalingv2.ExternalMetricSource{
								Metric: autoscalingv2.MetricIdentifier{
									Name: "external-1",
									Selector: &metav1.LabelSelector{
										MatchLabels: map[string]string{"x": "y"},
									},
								},
							},
						},
					},
				},
			},
			expectedCollector: "external-1",
		},
		{
			msg: "should get create collector type from type label (ignore legacy metric name)",
			hpa: &autoscalingv2.HorizontalPodAutoscaler{
				ObjectMeta: metav1.ObjectMeta{
					Annotations: map[string]string{},
				},
				Spec: autoscalingv2.HorizontalPodAutoscalerSpec{
					Metrics: []autoscalingv2.MetricSpec{
						{
							Type: autoscalingv2.ExternalMetricSourceType,
							External: &autoscalingv2.ExternalMetricSource{
								Metric: autoscalingv2.MetricIdentifier{
									Name: "external-1",
									Selector: &metav1.LabelSelector{
										MatchLabels: map[string]string{"type": "external-2"},
									},
								},
							},
						},
					},
				},
			},
			expectedCollector: "external-2",
		},
		{
			msg: "should not find collector when no collector matches",
			hpa: &autoscalingv2.HorizontalPodAutoscaler{
				ObjectMeta: metav1.ObjectMeta{
					Annotations: map[string]string{},
				},
				Spec: autoscalingv2.HorizontalPodAutoscalerSpec{
					Metrics: []autoscalingv2.MetricSpec{
						{
							Type: autoscalingv2.ExternalMetricSourceType,
							External: &autoscalingv2.ExternalMetricSource{
								Metric: autoscalingv2.MetricIdentifier{
									Name: "external-3",
								},
							},
						},
					},
				},
			},
			expectedCollector: "",
		},
	} {
		t.Run(tc.msg, func(t *testing.T) {
			collectorFactory := NewCollectorFactory()
			for _, collector := range []string{"1", "2"} {
				collectorFactory.RegisterExternalCollector([]string{"external-" + collector}, &mockCollectorPlugin{Name: "external-" + collector})
			}
			configs, err := ParseHPAMetrics(tc.hpa)
			require.NoError(t, err)
			require.Len(t, configs, 1)

			collector, err := collectorFactory.NewCollector(context.Background(), tc.hpa, configs[0], 0)
			if tc.expectedCollector == "" {
				require.Error(t, err)
			} else {
				require.NoError(t, err)

				c, ok := collector.(*mockCollector)
				require.True(t, ok)
				require.Equal(t, tc.expectedCollector, c.Name)
			}
		})
	}
}
