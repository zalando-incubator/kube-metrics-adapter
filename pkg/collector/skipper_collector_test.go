package collector

import (
	"encoding/json"
	"fmt"
	"github.com/stretchr/testify/assert"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/api/extensions/v1beta1"
	"k8s.io/apimachinery/pkg/api/resource"
	"k8s.io/client-go/kubernetes"
	"k8s.io/metrics/pkg/apis/custom_metrics"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"k8s.io/api/apps/v1"
	autoscalingv2beta1 "k8s.io/api/autoscaling/v2beta1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes/fake"
)

const (
	testBackendWeightsAnnotation  = "zalando.org/backend-weights"
	testStacksetWeightsAnnotation = "zalando.org/stack-set-weights"
)

func TestTargetRefReplicasDeployments(t *testing.T) {
	client := fake.NewSimpleClientset()
	name := "some-app"
	defaultNamespace := "default"
	deployment, err := newDeployment(client, defaultNamespace, name, 2, 1)
	require.NoError(t, err)

	// Create an HPA with the deployment as ref
	hpa, err := client.AutoscalingV2beta1().HorizontalPodAutoscalers(deployment.Namespace).
		Create(newHPA(defaultNamespace, name, "Deployment"))
	require.NoError(t, err)

	replicas, err := targetRefReplicas(client, hpa)
	require.NoError(t, err)
	require.Equal(t, deployment.Status.Replicas, replicas)
}

func TestTargetRefReplicasStatefulSets(t *testing.T) {
	client := fake.NewSimpleClientset()
	name := "some-app"
	defaultNamespace := "default"
	statefulSet, err := newStatefulSet(client, defaultNamespace, name)
	require.NoError(t, err)

	// Create an HPA with the statefulSet as ref
	hpa, err := client.AutoscalingV2beta1().HorizontalPodAutoscalers(statefulSet.Namespace).
		Create(newHPA(defaultNamespace, name, "StatefulSet"))
	require.NoError(t, err)

	replicas, err := targetRefReplicas(client, hpa)
	require.NoError(t, err)
	require.Equal(t, statefulSet.Status.Replicas, replicas)
}

func newHPA(namesapce string, refName string, refKind string) *autoscalingv2beta1.HorizontalPodAutoscaler {
	return &autoscalingv2beta1.HorizontalPodAutoscaler{
		ObjectMeta: metav1.ObjectMeta{
			Name: namesapce,
		},
		Spec: autoscalingv2beta1.HorizontalPodAutoscalerSpec{
			ScaleTargetRef: autoscalingv2beta1.CrossVersionObjectReference{
				Name: refName,
				Kind: refKind,
			},
		},
		Status: autoscalingv2beta1.HorizontalPodAutoscalerStatus{},
	}
}

func newDeployment(client *fake.Clientset, namespace string, name string, replicas, readyReplicas int32) (*v1.Deployment, error) {
	return client.AppsV1().Deployments(namespace).Create(&v1.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: namespace,
		},
		Spec: v1.DeploymentSpec{},
		Status: v1.DeploymentStatus{
			ReadyReplicas: replicas,
			Replicas:      readyReplicas,
		},
	})
}

func newStatefulSet(client *fake.Clientset, namespace string, name string) (*v1.StatefulSet, error) {
	return client.AppsV1().StatefulSets(namespace).Create(&v1.StatefulSet{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: namespace,
		},
		Status: v1.StatefulSetStatus{
			ReadyReplicas: 1,
			Replicas:      2,
		},
	})
}

func TestSkipperCollector(t *testing.T) {
	for _, tc := range []struct {
		msg                string
		metrics            []int
		backend            string
		ingressName        string
		collectedMetric    int
		namespace          string
		backendWeights     map[string]map[string]int
		replicas           int32
		readyReplicas      int32
		backendAnnotations []string
	}{
		{
			msg:             "test unweighted hpa",
			metrics:         []int{1000, 1000, 2000},
			ingressName:     "dummy-ingress",
			collectedMetric: 2000,
			namespace:       "default",
			backend:         "dummy-backend",
			replicas:        1,
			readyReplicas:   1,
		},
		{
			msg:                "test weighted backend",
			metrics:            []int{100, 1500, 700},
			ingressName:        "dummy-ingress",
			collectedMetric:    600,
			namespace:          "default",
			backend:            "backend1",
			backendWeights:     map[string]map[string]int{testBackendWeightsAnnotation: {"backend2": 60, "backend1": 40}},
			replicas:           1,
			readyReplicas:      1,
			backendAnnotations: []string{testBackendWeightsAnnotation},
		},
		{
			msg:                "test multiple replicas",
			metrics:            []int{100, 1500, 700},
			ingressName:        "dummy-ingress",
			collectedMetric:    150,
			namespace:          "default",
			backend:            "backend1",
			backendWeights:     map[string]map[string]int{testBackendWeightsAnnotation: {"backend2": 50, "backend1": 50}},
			replicas:           5,
			readyReplicas:      5,
			backendAnnotations: []string{testBackendWeightsAnnotation},
		},
		{
			msg:                "test zero weight backends",
			metrics:            []int{100, 1500, 700},
			ingressName:        "dummy-ingress",
			collectedMetric:    0,
			namespace:          "default",
			backend:            "backend1",
			backendWeights:     map[string]map[string]int{testBackendWeightsAnnotation: {"backend2": 100, "backend1": 0}},
			replicas:           5,
			readyReplicas:      5,
			backendAnnotations: []string{testBackendWeightsAnnotation},
		},
		{
			msg:             "test multiple backend annotation",
			metrics:         []int{100, 1500, 700},
			ingressName:     "dummy-ingress",
			collectedMetric: 300,
			namespace:       "default",
			backend:         "backend1",
			backendWeights: map[string]map[string]int{
				testBackendWeightsAnnotation:  {"backend2": 20, "backend1": 80},
				testStacksetWeightsAnnotation: {"backend2": 0, "backend1": 100},
			},
			replicas:           5,
			readyReplicas:      5,
			backendAnnotations: []string{testBackendWeightsAnnotation, testStacksetWeightsAnnotation},
		},
		{
			msg:                "test backend is not set",
			metrics:            []int{100, 1500, 700},
			ingressName:        "dummy-ingress",
			collectedMetric:    1500,
			namespace:          "default",
			backend:            "",
			backendWeights:     map[string]map[string]int{testBackendWeightsAnnotation: {"backend2": 100, "backend1": 0}},
			replicas:           1,
			readyReplicas:      1,
			backendAnnotations: []string{testBackendWeightsAnnotation},
		},
	} {
		t.Run(tc.msg, func(tt *testing.T) {
			client := fake.NewSimpleClientset()
			makeIngress(client, tc.namespace, tc.ingressName, tc.backend, tc.backendWeights)
			plugin := makePlugin(tc.metrics)
			hpa := makeHPA(tc.ingressName, tc.backend)
			config := makeConfig(tc.backend)
			newDeployment(client, tc.namespace, tc.backend, tc.replicas, tc.readyReplicas)
			collector, err := NewSkipperCollector(client, plugin, hpa, config, time.Minute, tc.backendAnnotations, tc.backend)
			assert.NoError(tt, err, "failed to create skipper collector: %v", err)
			collected, err := collector.GetMetrics()
			assert.NoError(tt, err, "failed to collect metrics: %v", err)
			assert.Len(t, collected, 1, "the number of metrics returned is not 1")
			assert.EqualValues(t, tc.collectedMetric, collected[0].Custom.Value.Value(), "the returned metric is not expected value")
		})
	}
}

func makeIngress(client kubernetes.Interface, namespace, ingressName, backend string, backendWeights map[string]map[string]int) error {
	annotations := make(map[string]string)
	for anno, weights := range backendWeights {
		sWeights, err := json.Marshal(weights)
		if err != nil {
			return err
		}
		annotations[anno] = string(sWeights)
	}
	_, err := client.ExtensionsV1beta1().Ingresses(namespace).Create(&v1beta1.Ingress{
		ObjectMeta: metav1.ObjectMeta{
			Name:        ingressName,
			Annotations: annotations,
		},
		Spec: v1beta1.IngressSpec{
			Backend: &v1beta1.IngressBackend{
				ServiceName: backend,
			},
			TLS: nil,
			Rules: []v1beta1.IngressRule{
				{
					Host: "example.org",
				},
			},
		},
		Status: v1beta1.IngressStatus{
			LoadBalancer: corev1.LoadBalancerStatus{
				Ingress: nil,
			},
		},
	})
	return err
}

func makeHPA(ingressName, backend string) *autoscalingv2beta1.HorizontalPodAutoscaler {
	return &autoscalingv2beta1.HorizontalPodAutoscaler{
		Spec: autoscalingv2beta1.HorizontalPodAutoscalerSpec{
			ScaleTargetRef: autoscalingv2beta1.CrossVersionObjectReference{
				Kind: "Deployment",
				Name: backend,
			},
			Metrics: []autoscalingv2beta1.MetricSpec{
				{
					Type: autoscalingv2beta1.ObjectMetricSourceType,
					Object: &autoscalingv2beta1.ObjectMetricSource{
						Target:     autoscalingv2beta1.CrossVersionObjectReference{Name: ingressName, APIVersion: "extensions/v1", Kind: "Ingress"},
						MetricName: fmt.Sprintf("%s,%s", rpsMetricName, backend),
					},
				},
			},
		},
	}
}
func makeConfig(backend string) *MetricConfig {
	return &MetricConfig{
		MetricTypeName: MetricTypeName{Name: fmt.Sprintf("%s,%s", rpsMetricName, backend)},
	}
}

type FakeCollectorPlugin struct {
	metrics []CollectedMetric
}

type FakeCollector struct {
	metrics []CollectedMetric
}

func (c *FakeCollector) GetMetrics() ([]CollectedMetric, error) {
	return c.metrics, nil
}

func (FakeCollector) Interval() time.Duration {
	return time.Minute
}

func (p *FakeCollectorPlugin) NewCollector(hpa *autoscalingv2beta1.HorizontalPodAutoscaler, config *MetricConfig, interval time.Duration) (Collector, error) {
	return &FakeCollector{metrics: p.metrics}, nil
}

func makePlugin(metrics []int) CollectorPlugin {
	m := make([]CollectedMetric, len(metrics))
	for i, v := range metrics {
		m[i] = CollectedMetric{Custom: custom_metrics.MetricValue{Value: *resource.NewQuantity(int64(v), resource.DecimalSI)}}
	}
	return &FakeCollectorPlugin{metrics: m}
}
