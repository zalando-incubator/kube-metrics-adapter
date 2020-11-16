package collector

import (
	"testing"

	"github.com/stretchr/testify/require"
	autoscalingv2 "k8s.io/api/autoscaling/v2beta2"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func TestNewPrometheusCollector(t *testing.T) {
	for _, tc := range []struct {
		msg string
		hpa *autoscalingv2.HorizontalPodAutoscaler
		// config        *MetricConfig
		valid         bool
		expectedQuery string
	}{
		{
			msg: "valid external metric configuration should work",
			hpa: &autoscalingv2.HorizontalPodAutoscaler{
				ObjectMeta: metav1.ObjectMeta{
					Annotations: map[string]string{
						"metric-config.external.rps.prometheus/query": "sum(rate(rps[1m]))",
					},
				},
				Spec: autoscalingv2.HorizontalPodAutoscalerSpec{
					Metrics: []autoscalingv2.MetricSpec{
						{
							Type: autoscalingv2.ExternalMetricSourceType,
							External: &autoscalingv2.ExternalMetricSource{
								Metric: autoscalingv2.MetricIdentifier{
									Name: "rps",
									Selector: &metav1.LabelSelector{
										MatchLabels: map[string]string{"type": "prometheus"},
									},
								},
							},
						},
					},
				},
			},
			expectedQuery: "sum(rate(rps[1m]))",
		},
		{
			msg: "missing query for external metric should not work",
			hpa: &autoscalingv2.HorizontalPodAutoscaler{
				ObjectMeta: metav1.ObjectMeta{
					Annotations: map[string]string{
						"metric-config.external.rps.prometheus/not-query": "sum(rate(rps[1m]))",
					},
				},
				Spec: autoscalingv2.HorizontalPodAutoscalerSpec{
					Metrics: []autoscalingv2.MetricSpec{
						{
							Type: autoscalingv2.ExternalMetricSourceType,
							External: &autoscalingv2.ExternalMetricSource{
								Metric: autoscalingv2.MetricIdentifier{
									Name: "rps",
									Selector: &metav1.LabelSelector{
										MatchLabels: map[string]string{"type": "prometheus"},
									},
								},
							},
						},
					},
				},
			},
			expectedQuery: "",
		},
		{
			msg: "valid legacy external metric configuration should work",
			hpa: &autoscalingv2.HorizontalPodAutoscaler{
				ObjectMeta: metav1.ObjectMeta{
					Annotations: map[string]string{
						"metric-config.external.prometheus-query.prometheus/rps": "sum(rate(rps[1m]))",
					},
				},
				Spec: autoscalingv2.HorizontalPodAutoscalerSpec{
					Metrics: []autoscalingv2.MetricSpec{
						{
							Type: autoscalingv2.ExternalMetricSourceType,
							External: &autoscalingv2.ExternalMetricSource{
								Metric: autoscalingv2.MetricIdentifier{
									Name: PrometheusMetricNameLegacy,
									Selector: &metav1.LabelSelector{
										MatchLabels: map[string]string{prometheusQueryNameLabelKey: "rps"},
									},
								},
							},
						},
					},
				},
			},
			expectedQuery: "sum(rate(rps[1m]))",
		},
		{
			msg: "invalid legacy external metric configuration with wrong query name should not work",
			hpa: &autoscalingv2.HorizontalPodAutoscaler{
				ObjectMeta: metav1.ObjectMeta{
					Annotations: map[string]string{
						"metric-config.external.prometheus-query.prometheus/rps": "sum(rate(rps[1m]))",
					},
				},
				Spec: autoscalingv2.HorizontalPodAutoscalerSpec{
					Metrics: []autoscalingv2.MetricSpec{
						{
							Type: autoscalingv2.ExternalMetricSourceType,
							External: &autoscalingv2.ExternalMetricSource{
								Metric: autoscalingv2.MetricIdentifier{
									Name: PrometheusMetricNameLegacy,
									Selector: &metav1.LabelSelector{
										MatchLabels: map[string]string{prometheusQueryNameLabelKey: "not-rps"},
									},
								},
							},
						},
					},
				},
			},
			expectedQuery: "",
		},
		{
			msg: "invalid legacy external metric configuration with missing selector should not work",
			hpa: &autoscalingv2.HorizontalPodAutoscaler{
				ObjectMeta: metav1.ObjectMeta{
					Annotations: map[string]string{
						"metric-config.external.prometheus-query.prometheus/rps": "sum(rate(rps[1m]))",
					},
				},
				Spec: autoscalingv2.HorizontalPodAutoscalerSpec{
					Metrics: []autoscalingv2.MetricSpec{
						{
							Type: autoscalingv2.ExternalMetricSourceType,
							External: &autoscalingv2.ExternalMetricSource{
								Metric: autoscalingv2.MetricIdentifier{
									Name: PrometheusMetricNameLegacy,
								},
							},
						},
					},
				},
			},
			expectedQuery: "",
		},
		{
			msg: "invalid legacy external metric configuration with missing query name should not work",
			hpa: &autoscalingv2.HorizontalPodAutoscaler{
				ObjectMeta: metav1.ObjectMeta{
					Annotations: map[string]string{
						"metric-config.external.prometheus-query.prometheus/rps": "sum(rate(rps[1m]))",
					},
				},
				Spec: autoscalingv2.HorizontalPodAutoscalerSpec{
					Metrics: []autoscalingv2.MetricSpec{
						{
							Type: autoscalingv2.ExternalMetricSourceType,
							External: &autoscalingv2.ExternalMetricSource{
								Metric: autoscalingv2.MetricIdentifier{
									Name: PrometheusMetricNameLegacy,
									Selector: &metav1.LabelSelector{
										MatchLabels: map[string]string{"not-query-name": "not-rps"},
									},
								},
							},
						},
					},
				},
			},
			expectedQuery: "",
		},
	} {
		t.Run(tc.msg, func(t *testing.T) {
			collectorFactory := NewCollectorFactory()
			promPlugin, err := NewPrometheusCollectorPlugin(nil, "http://prometheus")
			require.NoError(t, err)
			collectorFactory.RegisterExternalCollector([]string{PrometheusMetricType, PrometheusMetricNameLegacy}, promPlugin)
			configs, err := ParseHPAMetrics(tc.hpa)
			require.NoError(t, err)
			require.Len(t, configs, 1)

			collector, err := collectorFactory.NewCollector(tc.hpa, configs[0], 0)
			if tc.expectedQuery != "" {
				require.NoError(t, err)
				c, ok := collector.(*PrometheusCollector)
				require.True(t, ok)
				require.Equal(t, tc.expectedQuery, c.query)
			} else {
				require.Error(t, err)
			}
		})
	}
}
