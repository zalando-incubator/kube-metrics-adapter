package collector

import (
	"encoding/json"
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	appsv1 "k8s.io/api/apps/v1"
	autoscalingv2 "k8s.io/api/autoscaling/v2beta2"
	corev1 "k8s.io/api/core/v1"
	v1beta1 "k8s.io/api/networking/v1beta1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/kubernetes/fake"
	"k8s.io/metrics/pkg/apis/custom_metrics"
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
	hpa, err := client.AutoscalingV2beta2().HorizontalPodAutoscalers(deployment.Namespace).
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
	hpa, err := client.AutoscalingV2beta2().HorizontalPodAutoscalers(statefulSet.Namespace).
		Create(newHPA(defaultNamespace, name, "StatefulSet"))
	require.NoError(t, err)

	replicas, err := targetRefReplicas(client, hpa)
	require.NoError(t, err)
	require.Equal(t, statefulSet.Status.Replicas, replicas)
}

func newHPA(namesapce string, refName string, refKind string) *autoscalingv2.HorizontalPodAutoscaler {
	return &autoscalingv2.HorizontalPodAutoscaler{
		ObjectMeta: metav1.ObjectMeta{
			Name: namesapce,
		},
		Spec: autoscalingv2.HorizontalPodAutoscalerSpec{
			ScaleTargetRef: autoscalingv2.CrossVersionObjectReference{
				Name: refName,
				Kind: refKind,
			},
		},
		Status: autoscalingv2.HorizontalPodAutoscalerStatus{},
	}
}

func newDeployment(client *fake.Clientset, namespace string, name string, replicas, readyReplicas int32) (*appsv1.Deployment, error) {
	return client.AppsV1().Deployments(namespace).Create(&appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: namespace,
		},
		Spec: appsv1.DeploymentSpec{},
		Status: appsv1.DeploymentStatus{
			ReadyReplicas: replicas,
			Replicas:      readyReplicas,
		},
	})
}

func newStatefulSet(client *fake.Clientset, namespace string, name string) (*appsv1.StatefulSet, error) {
	return client.AppsV1().StatefulSets(namespace).Create(&appsv1.StatefulSet{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: namespace,
		},
		Status: appsv1.StatefulSetStatus{
			ReadyReplicas: 1,
			Replicas:      2,
		},
	})
}

func TestSkipperCollector(t *testing.T) {
	for _, tc := range []struct {
		msg                string
		metric             int
		backend            string
		ingressName        string
		hostnames          []string
		expectedQuery      string
		collectedMetric    int
		expectError        bool
		fakedAverage       bool
		namespace          string
		backendWeights     map[string]map[string]int
		replicas           int32
		readyReplicas      int32
		backendAnnotations []string
	}{
		{
			msg:             "test unweighted hpa",
			metric:          1000,
			ingressName:     "dummy-ingress",
			hostnames:       []string{"example.org"},
			expectedQuery:   `scalar(sum(rate(skipper_serve_host_duration_seconds_count{host=~"example_org"}[1m])) * 1.0000)`,
			collectedMetric: 1000,
			namespace:       "default",
			backend:         "dummy-backend",
			replicas:        1,
			readyReplicas:   1,
		},
		{
			msg:                "test weighted backend",
			metric:             1000,
			ingressName:        "dummy-ingress",
			hostnames:          []string{"example.org"},
			expectedQuery:      `scalar(sum(rate(skipper_serve_host_duration_seconds_count{host=~"example_org"}[1m])) * 0.4000)`,
			collectedMetric:    1000,
			namespace:          "default",
			backend:            "backend1",
			backendWeights:     map[string]map[string]int{testBackendWeightsAnnotation: {"backend2": 60, "backend1": 40}},
			replicas:           1,
			readyReplicas:      1,
			backendAnnotations: []string{testBackendWeightsAnnotation},
		},
		{
			msg:                "test multiple hostnames",
			metric:             1000,
			ingressName:        "dummy-ingress",
			hostnames:          []string{"example.org", "foo.bar.com", "test.org"},
			expectedQuery:      `scalar(sum(rate(skipper_serve_host_duration_seconds_count{host=~"example_org|foo_bar_com|test_org"}[1m])) * 0.4000)`,
			collectedMetric:    1000,
			namespace:          "default",
			backend:            "backend1",
			backendWeights:     map[string]map[string]int{testBackendWeightsAnnotation: {"backend2": 60, "backend1": 40}},
			replicas:           1,
			readyReplicas:      1,
			backendAnnotations: []string{testBackendWeightsAnnotation},
		},
		{
			msg:                "test multiple replicas",
			metric:             1000,
			ingressName:        "dummy-ingress",
			hostnames:          []string{"example.org"},
			expectedQuery:      `scalar(sum(rate(skipper_serve_host_duration_seconds_count{host=~"example_org"}[1m])) * 0.5000)`,
			collectedMetric:    200,
			fakedAverage:       true,
			namespace:          "default",
			backend:            "backend1",
			backendWeights:     map[string]map[string]int{testBackendWeightsAnnotation: {"backend2": 50, "backend1": 50}},
			replicas:           5,
			readyReplicas:      5,
			backendAnnotations: []string{testBackendWeightsAnnotation},
		},
		{
			msg:                "test multiple replicas not calculating average internally",
			metric:             1500,
			ingressName:        "dummy-ingress",
			hostnames:          []string{"example.org"},
			expectedQuery:      `scalar(sum(rate(skipper_serve_host_duration_seconds_count{host=~"example_org"}[1m])) * 0.5000)`,
			collectedMetric:    1500,
			namespace:          "default",
			backend:            "backend1",
			backendWeights:     map[string]map[string]int{testBackendWeightsAnnotation: {"backend2": 50, "backend1": 50}},
			replicas:           5, // this is not taken into account
			readyReplicas:      5,
			backendAnnotations: []string{testBackendWeightsAnnotation},
		},
		{
			msg:                "test zero weight backends",
			metric:             0,
			ingressName:        "dummy-ingress",
			hostnames:          []string{"example.org"},
			expectedQuery:      `scalar(sum(rate(skipper_serve_host_duration_seconds_count{host=~"example_org"}[1m])) * 0.0000)`,
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
			metric:          1500,
			ingressName:     "dummy-ingress",
			hostnames:       []string{"example.org"},
			expectedQuery:   `scalar(sum(rate(skipper_serve_host_duration_seconds_count{host=~"example_org"}[1m])) * 1.0000)`,
			collectedMetric: 300,
			fakedAverage:    true,
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
			msg:             "test multiple backend annotation not calculating average internally",
			metric:          1500,
			ingressName:     "dummy-ingress",
			hostnames:       []string{"example.org"},
			expectedQuery:   `scalar(sum(rate(skipper_serve_host_duration_seconds_count{host=~"example_org"}[1m])) * 1.0000)`,
			collectedMetric: 1500,
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
			metric:             0,
			ingressName:        "dummy-ingress",
			hostnames:          []string{"example.org"},
			expectedQuery:      `scalar(sum(rate(skipper_serve_host_duration_seconds_count{host=~"example_org"}[1m])) * 0.0000)`,
			collectedMetric:    0,
			namespace:          "default",
			backend:            "backend3",
			backendWeights:     map[string]map[string]int{testBackendWeightsAnnotation: {"backend2": 100, "backend1": 0}},
			replicas:           1,
			readyReplicas:      1,
			backendAnnotations: []string{testBackendWeightsAnnotation},
		},
		{
			msg:                "test no annotations set",
			metric:             1500,
			ingressName:        "dummy-ingress",
			hostnames:          []string{"example.org"},
			expectedQuery:      `scalar(sum(rate(skipper_serve_host_duration_seconds_count{host=~"example_org"}[1m])) * 1.0000)`,
			collectedMetric:    1500,
			namespace:          "default",
			backend:            "backend3",
			backendWeights:     map[string]map[string]int{},
			replicas:           1,
			readyReplicas:      1,
			backendAnnotations: []string{testBackendWeightsAnnotation},
		},
		{
			msg:                "test annotations are set but backend is missing",
			metric:             1500,
			ingressName:        "dummy-ingress",
			hostnames:          []string{"example.org"},
			expectedQuery:      `scalar(sum(rate(skipper_serve_host_duration_seconds_count{host=~"example_org"}[1m])) * 1.0000)`,
			expectError:        true,
			namespace:          "default",
			backend:            "",
			backendWeights:     map[string]map[string]int{testBackendWeightsAnnotation: {"backend2": 100, "backend1": 0}},
			replicas:           1,
			readyReplicas:      1,
			backendAnnotations: []string{testBackendWeightsAnnotation},
		},
		{
			msg:                "test annotations are missing and backend is unset",
			metric:             1500,
			ingressName:        "dummy-ingress",
			hostnames:          []string{"example.org"},
			expectedQuery:      `scalar(sum(rate(skipper_serve_host_duration_seconds_count{host=~"example_org"}[1m])) * 1.0000)`,
			collectedMetric:    1500,
			namespace:          "default",
			backend:            "",
			backendWeights:     nil,
			replicas:           1,
			readyReplicas:      1,
			backendAnnotations: []string{testBackendWeightsAnnotation},
		},
		{
			msg:             "test partial backend annotations",
			metric:          1500,
			ingressName:     "dummy-ingress",
			hostnames:       []string{"example.org"},
			expectedQuery:   `scalar(sum(rate(skipper_serve_host_duration_seconds_count{host=~"example_org"}[1m])) * 0.2000)`,
			collectedMetric: 300,
			fakedAverage:    true,
			namespace:       "default",
			backend:         "backend2",
			backendWeights: map[string]map[string]int{
				testBackendWeightsAnnotation:  {"backend2": 20, "backend1": 80},
				testStacksetWeightsAnnotation: {"backend1": 100},
			},
			replicas:           5,
			readyReplicas:      5,
			backendAnnotations: []string{testBackendWeightsAnnotation, testStacksetWeightsAnnotation},
		},
		{
			msg:             "test partial backend annotations not calculating average internally",
			metric:          1500,
			ingressName:     "dummy-ingress",
			hostnames:       []string{"example.org"},
			expectedQuery:   `scalar(sum(rate(skipper_serve_host_duration_seconds_count{host=~"example_org"}[1m])) * 0.2000)`,
			collectedMetric: 1500,
			namespace:       "default",
			backend:         "backend2",
			backendWeights: map[string]map[string]int{
				testBackendWeightsAnnotation:  {"backend2": 20, "backend1": 80},
				testStacksetWeightsAnnotation: {"backend1": 100},
			},
			replicas:           5,
			readyReplicas:      5,
			backendAnnotations: []string{testBackendWeightsAnnotation, testStacksetWeightsAnnotation},
		},
	} {
		t.Run(tc.msg, func(t *testing.T) {
			client := fake.NewSimpleClientset()
			err := makeIngress(client, tc.namespace, tc.ingressName, tc.backend, tc.hostnames, tc.backendWeights)
			require.NoError(t, err)
			plugin := makePlugin(tc.metric)
			hpa := makeHPA(tc.namespace, tc.ingressName, tc.backend)
			config := makeConfig(tc.ingressName, tc.namespace, tc.backend, tc.fakedAverage)
			_, err = newDeployment(client, tc.namespace, tc.backend, tc.replicas, tc.readyReplicas)
			require.NoError(t, err)
			collector, err := NewSkipperCollector(client, plugin, hpa, config, time.Minute, tc.backendAnnotations, tc.backend)
			require.NoError(t, err, "failed to create skipper collector: %v", err)
			collected, err := collector.GetMetrics()
			if tc.expectError {
				require.Error(t, err)
			} else {
				require.NoError(t, err)
				require.Equal(t, map[string]string{"query": tc.expectedQuery}, plugin.config)
				require.NoError(t, err, "failed to collect metrics: %v", err)
				require.Len(t, collected, 1, "the number of metrics returned is not 1")
				require.EqualValues(t, tc.collectedMetric, collected[0].Custom.Value.Value(), "the returned metric is not expected value")
			}
		})
	}
}

func makeIngress(client kubernetes.Interface, namespace, ingressName, backend string, hostnames []string, backendWeights map[string]map[string]int) error {
	annotations := make(map[string]string)
	for anno, weights := range backendWeights {
		sWeights, err := json.Marshal(weights)
		if err != nil {
			return err
		}
		annotations[anno] = string(sWeights)
	}
	ingress := &v1beta1.Ingress{
		ObjectMeta: metav1.ObjectMeta{
			Name:        ingressName,
			Annotations: annotations,
		},
		Spec: v1beta1.IngressSpec{
			Backend: &v1beta1.IngressBackend{
				ServiceName: backend,
			},
			TLS: nil,
		},
		Status: v1beta1.IngressStatus{
			LoadBalancer: corev1.LoadBalancerStatus{
				Ingress: nil,
			},
		},
	}
	for _, hostname := range hostnames {
		ingress.Spec.Rules = append(ingress.Spec.Rules, v1beta1.IngressRule{
			Host: hostname,
		})
	}
	_, err := client.NetworkingV1beta1().Ingresses(namespace).Create(ingress)
	return err
}

func makeHPA(namespace, ingressName, backend string) *autoscalingv2.HorizontalPodAutoscaler {
	return &autoscalingv2.HorizontalPodAutoscaler{
		ObjectMeta: metav1.ObjectMeta{Namespace: namespace},
		Spec: autoscalingv2.HorizontalPodAutoscalerSpec{
			ScaleTargetRef: autoscalingv2.CrossVersionObjectReference{
				Kind: "Deployment",
				Name: backend,
			},
			Metrics: []autoscalingv2.MetricSpec{
				{
					Type: autoscalingv2.ObjectMetricSourceType,
					Object: &autoscalingv2.ObjectMetricSource{
						DescribedObject: autoscalingv2.CrossVersionObjectReference{Name: ingressName, APIVersion: "extensions/v1", Kind: "Ingress"},
						Metric:          autoscalingv2.MetricIdentifier{Name: fmt.Sprintf("%s,%s", rpsMetricName, backend)},
					},
				},
			},
		},
	}
}
func makeConfig(ingressName, namespace, backend string, fakedAverage bool) *MetricConfig {
	config := &MetricConfig{
		MetricTypeName: MetricTypeName{Metric: autoscalingv2.MetricIdentifier{Name: fmt.Sprintf("%s,%s", rpsMetricName, backend)}},
		ObjectReference: custom_metrics.ObjectReference{
			Name:      ingressName,
			Namespace: namespace,
		},
		MetricSpec: autoscalingv2.MetricSpec{
			Object: &autoscalingv2.ObjectMetricSource{
				Target: autoscalingv2.MetricTarget{},
			},
		},
	}

	if fakedAverage {
		config.MetricSpec.Object.Target.Value = resource.NewQuantity(10, resource.DecimalSI)
	} else {
		config.MetricSpec.Object.Target.AverageValue = resource.NewQuantity(10, resource.DecimalSI)
	}
	return config
}

type FakeCollectorPlugin struct {
	metrics []CollectedMetric
	config  map[string]string
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

func (p *FakeCollectorPlugin) NewCollector(hpa *autoscalingv2.HorizontalPodAutoscaler, config *MetricConfig, interval time.Duration) (Collector, error) {
	if p.config != nil {
		return nil, fmt.Errorf("config already assigned once: %v", p.config)
	}
	p.config = config.Config
	return &FakeCollector{metrics: p.metrics}, nil
}

func makePlugin(metric int) *FakeCollectorPlugin {
	return &FakeCollectorPlugin{
		metrics: []CollectedMetric{
			{
				Custom: custom_metrics.MetricValue{Value: *resource.NewQuantity(int64(metric), resource.DecimalSI)},
			},
		},
	}
}
