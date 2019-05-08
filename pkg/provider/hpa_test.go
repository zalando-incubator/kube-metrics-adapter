package provider

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"github.com/zalando-incubator/kube-metrics-adapter/pkg/collector"
	autoscalingv2 "k8s.io/api/autoscaling/v2beta2"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes/fake"
)

type mockCollectorPlugin struct{}

func (m mockCollectorPlugin) NewCollector(hpa *autoscalingv2.HorizontalPodAutoscaler, config *collector.MetricConfig, interval time.Duration) (collector.Collector, error) {
	return mockCollector{}, nil
}

type mockCollector struct{}

func (c mockCollector) GetMetrics() ([]collector.CollectedMetric, error) {
	return nil, nil
}

func (c mockCollector) Interval() time.Duration {
	return 1 * time.Second
}

func TestUpdateHPAs(t *testing.T) {
	value := resource.MustParse("1k")
	hpa := &autoscalingv2.HorizontalPodAutoscaler{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "hpa1",
			Namespace: "default",
			Annotations: map[string]string{
				"metric-config.pods.requests-per-second.json-path/json-key": "$.http_server.rps",
				"metric-config.pods.requests-per-second.json-path/path":     "/metrics",
				"metric-config.pods.requests-per-second.json-path/port":     "9090",
			},
		},
		Spec: autoscalingv2.HorizontalPodAutoscalerSpec{
			ScaleTargetRef: autoscalingv2.CrossVersionObjectReference{
				Kind:       "Deployment",
				Name:       "app",
				APIVersion: "apps/v1",
			},
			MinReplicas: &[]int32{1}[0],
			MaxReplicas: 10,
			Metrics: []autoscalingv2.MetricSpec{
				{
					Type: autoscalingv2.PodsMetricSourceType,
					Pods: &autoscalingv2.PodsMetricSource{
						Metric: autoscalingv2.MetricIdentifier{
							Name: "requests-per-second",
						},
						Target: autoscalingv2.MetricTarget{
							Type:         autoscalingv2.AverageValueMetricType,
							AverageValue: &value,
						},
					},
				},
			},
		},
	}

	fakeClient := fake.NewSimpleClientset()

	var err error
	hpa, err = fakeClient.AutoscalingV2beta2().HorizontalPodAutoscalers("default").Create(hpa)
	require.NoError(t, err)

	collectorFactory := collector.NewCollectorFactory()
	err = collectorFactory.RegisterPodsCollector("", mockCollectorPlugin{})
	require.NoError(t, err)

	provider := NewHPAProvider(fakeClient, 1*time.Second, 1*time.Second, collectorFactory)
	provider.collectorScheduler = NewCollectorScheduler(context.Background(), provider.metricSink)

	err = provider.updateHPAs()
	require.NoError(t, err)
	require.Len(t, provider.collectorScheduler.table, 1)

	// update HPA
	hpa.Annotations["metric-config.pods.requests-per-second.json-path/port"] = "8080"
	_, err = fakeClient.AutoscalingV2beta2().HorizontalPodAutoscalers("default").Update(hpa)
	require.NoError(t, err)

	err = provider.updateHPAs()
	require.NoError(t, err)

	require.Len(t, provider.collectorScheduler.table, 1)
}
