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

func TestMetricTypeName_String(t *testing.T) {
	for _, tc := range []struct {
		name          string
		metricType    autoscalingv2.MetricSourceType
		metricName    string
		selector      *metav1.LabelSelector
		expectedStr   string
		shouldContain []string
	}{
		{
			name:        "nil selector with PodsMetricSourceType",
			metricType:  autoscalingv2.PodsMetricSourceType,
			metricName:  "metric-name",
			selector:    nil,
			expectedStr: "Pods/metric-name",
		},
		{
			name:        "nil selector with ObjectMetricSourceType",
			metricType:  autoscalingv2.ObjectMetricSourceType,
			metricName:  "metric-name",
			selector:    nil,
			expectedStr: "Object/metric-name",
		},
		{
			name:        "nil selector with ExternalMetricSourceType",
			metricType:  autoscalingv2.ExternalMetricSourceType,
			metricName:  "metric-name",
			selector:    nil,
			expectedStr: "External/metric-name",
		},
		{
			name:       "empty selector (non-nil but empty MatchLabels)",
			metricType: autoscalingv2.ExternalMetricSourceType,
			metricName: "metric-name",
			selector: &metav1.LabelSelector{
				MatchLabels: map[string]string{},
			},
			expectedStr: "External/metric-name",
		},
		{
			name:       "selector with single MatchLabel",
			metricType: autoscalingv2.ExternalMetricSourceType,
			metricName: "metric-name",
			selector: &metav1.LabelSelector{
				MatchLabels: map[string]string{"key": "value"},
			},
			shouldContain: []string{"External/metric-name", "key=value"},
		},
		{
			name:       "selector with multiple MatchLabels",
			metricType: autoscalingv2.ExternalMetricSourceType,
			metricName: "metric-name",
			selector: &metav1.LabelSelector{
				MatchLabels: map[string]string{"key1": "val1", "key2": "val2"},
			},
			shouldContain: []string{"External/metric-name", "key1=val1", "key2=val2"},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			mtn := MetricTypeName{
				Type: tc.metricType,
				Metric: autoscalingv2.MetricIdentifier{
					Name:     tc.metricName,
					Selector: tc.selector,
				},
			}

			result := mtn.String()

			if tc.expectedStr != "" {
				require.Equal(t, tc.expectedStr, result)
			}

			if len(tc.shouldContain) > 0 {
				for _, substring := range tc.shouldContain {
					require.Contains(t, result, substring, "result should contain %q", substring)
				}
			}
		})
	}
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
