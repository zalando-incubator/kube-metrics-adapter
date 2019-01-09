package collector

import (
	"encoding/json"
	"fmt"
	"github.com/stretchr/testify/assert"
	autoscalingv2beta2 "k8s.io/api/autoscaling/v2beta2"
	"k8s.io/api/extensions/v1beta1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes/fake"
	"k8s.io/metrics/pkg/apis/custom_metrics"
	"testing"
	"time"
)

type testCollector struct {
	metrics  []int
	interval time.Duration
}

func (c testCollector) GetMetrics() ([]CollectedMetric, error) {
	return []CollectedMetric{
		{
			Custom: custom_metrics.MetricValue{Value: *resource.NewQuantity(1000, resource.DecimalSI)},
		},
	}, nil
}

func (c testCollector) Interval() time.Duration {
	return c.interval
}

type testCollectorPlugin struct {
	metrics []int
}

func (c testCollectorPlugin) NewCollector(hpa *autoscalingv2beta2.HorizontalPodAutoscaler, config *MetricConfig, interval time.Duration) (Collector, error) {
	return &testCollector{interval: interval, metrics: c.metrics}, nil
}

func makeTestCollectorPlugin(metrics []int) *testCollectorPlugin {
	return &testCollectorPlugin{metrics: metrics}
}

func makeHpa(ingressName string) *autoscalingv2beta2.HorizontalPodAutoscaler {
	return &autoscalingv2beta2.HorizontalPodAutoscaler{
		Spec: autoscalingv2beta2.HorizontalPodAutoscalerSpec{
			Metrics: []autoscalingv2beta2.MetricSpec{
				{
					Type: autoscalingv2beta2.ObjectMetricSourceType,
					Object: &autoscalingv2beta2.ObjectMetricSource{
						DescribedObject: autoscalingv2beta2.CrossVersionObjectReference{
							Kind:       "Ingress",
							APIVersion: "extensions/v1",
							Name:       ingressName,
						},
						Metric: autoscalingv2beta2.MetricIdentifier{
							Name: rpsMetricName,
						},
						Target: autoscalingv2beta2.MetricTarget{},
					},
				},
			},
		},
	}
}

func makeMetricConfig(ingressName, backend string) *MetricConfig {
	selector, _ := metav1.ParseToLabelSelector(fmt.Sprintf("%s=%s", hostLabel, backend))
	return &MetricConfig{
		ObjectReference: custom_metrics.ObjectReference{Kind: "Ingress", Namespace: "default", Name: ingressName},
		MetricTypeName: MetricTypeName{
			Name:         rpsMetricName,
			MetricLabels: selector,
		},
	}
}

func makeIngress(client *fake.Clientset, ingressName string, bWeights, sWeights map[string]int) {
	backendWeightAnno, _ := json.Marshal(bWeights)
	stacksetWeightAnno, _ := json.Marshal(sWeights)
	client.Extensions().Ingresses("default").Create(&v1beta1.Ingress{
		ObjectMeta: metav1.ObjectMeta{
			Name: ingressName,
			Annotations: map[string]string{
				stackTrafficWeights: string(stacksetWeightAnno),
				backendWeights:      string(backendWeightAnno),
			},
		},
		Spec: v1beta1.IngressSpec{
			Rules: []v1beta1.IngressRule{
				{
					Host: "dummy.example.com",
				},
			},
		},
	})
}

func TestNewSkipperCollectorSimple(t *testing.T) {
	for _, tc := range []struct {
		bWeights       map[string]int
		sWeights       map[string]int
		metrics        []int
		msg            string
		ingressName    string
		backend        string
		expectedMetric int
	}{
		{
			msg:            "backend without weights",
			sWeights:       map[string]int{"another-backend": 60},
			bWeights:       map[string]int{"another-backend": 60},
			ingressName:    "dummy-ingress",
			backend:        "dummy-backend",
			metrics:        []int{1000},
			expectedMetric: 1000,
		},
		{
			msg:            "weighted backed",
			sWeights:       map[string]int{"dummy-backend": 60},
			bWeights:       map[string]int{"dummy-backend": 60},
			ingressName:    "dummy-ingress",
			backend:        "dummy-backend",
			metrics:        []int{1000},
			expectedMetric: 600,
		},
		{
			msg:            "missing weights annotation",
			ingressName:    "dummy-ingress",
			backend:        "dummy-backend",
			metrics:        []int{1000},
			expectedMetric: 1000,
		},
		{
			msg:            "missing stackset weights annotation",
			bWeights:       map[string]int{"dummy-backend": 40},
			ingressName:    "dummy-ingress",
			backend:        "dummy-backend",
			metrics:        []int{1000},
			expectedMetric: 400,
		},
		{
			msg:            "missing backend weights annotation",
			sWeights:       map[string]int{"dummy-backend": 100},
			ingressName:    "dummy-ingress",
			backend:        "dummy-backend",
			metrics:        []int{1000},
			expectedMetric: 1000,
		},
	} {
		t.Run(tc.msg, func(tt *testing.T) {
			client := fake.NewSimpleClientset()
			makeIngress(client, tc.ingressName, tc.bWeights, tc.sWeights)
			plugin := makeTestCollectorPlugin(tc.metrics)
			hpa := makeHpa(tc.ingressName)
			config := makeMetricConfig(tc.ingressName, tc.backend)
			collector, err := NewSkipperCollector(client, plugin, hpa, config, time.Minute)
			assert.NoError(t, err, "failed to create skipper collector: %v", err)
			collectedMetrics, err := collector.GetMetrics()
			assert.NoError(t, err, "failed to get metrics from collector: %v", err)
			assert.Len(t, collectedMetrics, len(tc.metrics), "number of metrics returned is not %d", len(tc.metrics))
			assert.EqualValues(t, collectedMetrics[0].Custom.Value.Value(), tc.expectedMetric, "the collected metric is not 1000")
		})
	}
}
