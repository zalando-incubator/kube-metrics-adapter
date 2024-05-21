package collector

import (
	"context"
	"encoding/json"
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	rgv1 "github.com/szuecs/routegroup-client/apis/zalando.org/v1"
	rginterface "github.com/szuecs/routegroup-client/client/clientset/versioned"
	rgfake "github.com/szuecs/routegroup-client/client/clientset/versioned/fake"
	appsv1 "k8s.io/api/apps/v1"
	autoscalingv2 "k8s.io/api/autoscaling/v2"
	netv1 "k8s.io/api/networking/v1"
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
	hpa, err := client.AutoscalingV2().HorizontalPodAutoscalers(deployment.Namespace).
		Create(context.TODO(), newHPA(defaultNamespace, name, "Deployment"), metav1.CreateOptions{})
	require.NoError(t, err)

	replicas, err := targetRefReplicas(context.Background(), client, hpa)
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
	hpa, err := client.AutoscalingV2().HorizontalPodAutoscalers(statefulSet.Namespace).
		Create(context.TODO(), newHPA(defaultNamespace, name, "StatefulSet"), metav1.CreateOptions{})
	require.NoError(t, err)

	replicas, err := targetRefReplicas(context.Background(), client, hpa)
	require.NoError(t, err)
	require.Equal(t, statefulSet.Status.Replicas, replicas)
}

func newHPA(namespace string, refName string, refKind string) *autoscalingv2.HorizontalPodAutoscaler {
	return &autoscalingv2.HorizontalPodAutoscaler{
		ObjectMeta: metav1.ObjectMeta{
			Name: namespace,
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
	return client.AppsV1().Deployments(namespace).Create(context.TODO(), &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: namespace,
		},
		Spec: appsv1.DeploymentSpec{},
		Status: appsv1.DeploymentStatus{
			ReadyReplicas: replicas,
			Replicas:      readyReplicas,
		},
	}, metav1.CreateOptions{})
}

func newStatefulSet(client *fake.Clientset, namespace string, name string) (*appsv1.StatefulSet, error) {
	return client.AppsV1().StatefulSets(namespace).Create(context.TODO(), &appsv1.StatefulSet{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: namespace,
		},
		Status: appsv1.StatefulSetStatus{
			ReadyReplicas: 1,
			Replicas:      2,
		},
	}, metav1.CreateOptions{})
}

func TestSkipperCollectorIngress(t *testing.T) {
	for _, tc := range []struct {
		msg                string
		metric             int
		backend            string
		resourceName       string
		hostnames          []string
		expectedQuery      string
		collectedMetric    int
		expectError        bool
		fakedAverage       bool
		namespace          string
		backendWeights     map[string]map[string]float64
		replicas           int32
		readyReplicas      int32
		backendAnnotations []string
	}{
		{
			msg:             "test unweighted hpa",
			metric:          1000,
			resourceName:    "dummy-ingress",
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
			resourceName:       "dummy-ingress",
			hostnames:          []string{"example.org"},
			expectedQuery:      `scalar(sum(rate(skipper_serve_host_duration_seconds_count{host=~"example_org"}[1m])) * 0.4000)`,
			collectedMetric:    1000,
			namespace:          "default",
			backend:            "backend1",
			backendWeights:     map[string]map[string]float64{testBackendWeightsAnnotation: {"backend2": 60.0, "backend1": 40}},
			replicas:           1,
			readyReplicas:      1,
			backendAnnotations: []string{testBackendWeightsAnnotation},
		},
		{
			msg:                "test multiple hostnames",
			metric:             1000,
			resourceName:       "dummy-ingress",
			hostnames:          []string{"example.org", "foo.bar.com", "test.org"},
			expectedQuery:      `scalar(sum(rate(skipper_serve_host_duration_seconds_count{host=~"example_org|foo_bar_com|test_org"}[1m])) * 0.4000)`,
			collectedMetric:    1000,
			namespace:          "default",
			backend:            "backend1",
			backendWeights:     map[string]map[string]float64{testBackendWeightsAnnotation: {"backend2": 60, "backend1": 40}},
			replicas:           1,
			readyReplicas:      1,
			backendAnnotations: []string{testBackendWeightsAnnotation},
		},
		{
			msg:                "test multiple replicas",
			metric:             1000,
			resourceName:       "dummy-ingress",
			hostnames:          []string{"example.org"},
			expectedQuery:      `scalar(sum(rate(skipper_serve_host_duration_seconds_count{host=~"example_org"}[1m])) * 0.5000)`,
			collectedMetric:    200,
			fakedAverage:       true,
			namespace:          "default",
			backend:            "backend1",
			backendWeights:     map[string]map[string]float64{testBackendWeightsAnnotation: {"backend2": 50, "backend1": 50}},
			replicas:           5,
			readyReplicas:      5,
			backendAnnotations: []string{testBackendWeightsAnnotation},
		},
		{
			msg:                "test multiple replicas not calculating average internally",
			metric:             1500,
			resourceName:       "dummy-ingress",
			hostnames:          []string{"example.org"},
			expectedQuery:      `scalar(sum(rate(skipper_serve_host_duration_seconds_count{host=~"example_org"}[1m])) * 0.5000)`,
			collectedMetric:    1500,
			namespace:          "default",
			backend:            "backend1",
			backendWeights:     map[string]map[string]float64{testBackendWeightsAnnotation: {"backend2": 50, "backend1": 50}},
			replicas:           5, // this is not taken into account
			readyReplicas:      5,
			backendAnnotations: []string{testBackendWeightsAnnotation},
		},
		{
			msg:                "test zero weight backends",
			metric:             0,
			resourceName:       "dummy-ingress",
			hostnames:          []string{"example.org"},
			expectedQuery:      `scalar(sum(rate(skipper_serve_host_duration_seconds_count{host=~"example_org"}[1m])) * 0.0000)`,
			collectedMetric:    0,
			namespace:          "default",
			backend:            "backend1",
			backendWeights:     map[string]map[string]float64{testBackendWeightsAnnotation: {"backend2": 100, "backend1": 0}},
			replicas:           5,
			readyReplicas:      5,
			backendAnnotations: []string{testBackendWeightsAnnotation},
		},
		{
			msg:             "test multiple backend annotation",
			metric:          1500,
			resourceName:    "dummy-ingress",
			hostnames:       []string{"example.org"},
			expectedQuery:   `scalar(sum(rate(skipper_serve_host_duration_seconds_count{host=~"example_org"}[1m])) * 1.0000)`,
			collectedMetric: 300,
			fakedAverage:    true,
			namespace:       "default",
			backend:         "backend1",
			backendWeights: map[string]map[string]float64{
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
			resourceName:    "dummy-ingress",
			hostnames:       []string{"example.org"},
			expectedQuery:   `scalar(sum(rate(skipper_serve_host_duration_seconds_count{host=~"example_org"}[1m])) * 1.0000)`,
			collectedMetric: 1500,
			namespace:       "default",
			backend:         "backend1",
			backendWeights: map[string]map[string]float64{
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
			resourceName:       "dummy-ingress",
			hostnames:          []string{"example.org"},
			expectedQuery:      `scalar(sum(rate(skipper_serve_host_duration_seconds_count{host=~"example_org"}[1m])) * 0.0000)`,
			collectedMetric:    0,
			namespace:          "default",
			backend:            "backend3",
			backendWeights:     map[string]map[string]float64{testBackendWeightsAnnotation: {"backend2": 100, "backend1": 0}},
			replicas:           1,
			readyReplicas:      1,
			backendAnnotations: []string{testBackendWeightsAnnotation},
		},
		{
			msg:                "test no annotations set",
			metric:             1500,
			resourceName:       "dummy-ingress",
			hostnames:          []string{"example.org"},
			expectedQuery:      `scalar(sum(rate(skipper_serve_host_duration_seconds_count{host=~"example_org"}[1m])) * 1.0000)`,
			collectedMetric:    1500,
			namespace:          "default",
			backend:            "backend3",
			backendWeights:     map[string]map[string]float64{},
			replicas:           1,
			readyReplicas:      1,
			backendAnnotations: []string{testBackendWeightsAnnotation},
		},
		{
			msg:                "test annotations are set but backend is missing",
			metric:             1500,
			resourceName:       "dummy-ingress",
			hostnames:          []string{"example.org"},
			expectedQuery:      `scalar(sum(rate(skipper_serve_host_duration_seconds_count{host=~"example_org"}[1m])) * 1.0000)`,
			expectError:        true,
			namespace:          "default",
			backend:            "",
			backendWeights:     map[string]map[string]float64{testBackendWeightsAnnotation: {"backend2": 100, "backend1": 0}},
			replicas:           1,
			readyReplicas:      1,
			backendAnnotations: []string{testBackendWeightsAnnotation},
		},
		{
			msg:                "test annotations are missing and backend is unset",
			metric:             1500,
			resourceName:       "dummy-ingress",
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
			resourceName:    "dummy-ingress",
			hostnames:       []string{"example.org"},
			expectedQuery:   `scalar(sum(rate(skipper_serve_host_duration_seconds_count{host=~"example_org"}[1m])) * 0.2000)`,
			collectedMetric: 300,
			fakedAverage:    true,
			namespace:       "default",
			backend:         "backend2",
			backendWeights: map[string]map[string]float64{
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
			resourceName:    "dummy-ingress",
			hostnames:       []string{"example.org"},
			expectedQuery:   `scalar(sum(rate(skipper_serve_host_duration_seconds_count{host=~"example_org"}[1m])) * 0.2050)`,
			collectedMetric: 1500,
			namespace:       "default",
			backend:         "backend2",
			backendWeights: map[string]map[string]float64{
				testBackendWeightsAnnotation:  {"backend2": 20.5, "backend1": 79.5},
				testStacksetWeightsAnnotation: {"backend1": 100},
			},
			replicas:           5,
			readyReplicas:      5,
			backendAnnotations: []string{testBackendWeightsAnnotation, testStacksetWeightsAnnotation},
		},
	} {
		t.Run(tc.msg, func(t *testing.T) {
			client := fake.NewSimpleClientset()
			err := makeIngress(client, tc.namespace, tc.resourceName, tc.backend, tc.hostnames, tc.backendWeights)
			require.NoError(t, err)
			hpa := makeIngressHPA(tc.namespace, tc.resourceName, tc.backend)
			_, err = newDeployment(client, tc.namespace, tc.backend, tc.replicas, tc.readyReplicas)
			plugin := makePlugin(tc.metric)
			config := makeConfig(tc.resourceName, tc.namespace, hpa.Spec.Metrics[0].Object.DescribedObject.Kind, tc.backend, tc.fakedAverage)
			require.NoError(t, err)
			collector, err := NewSkipperCollector(client, nil, plugin, hpa, config, time.Minute, tc.backendAnnotations, tc.backend)
			require.NoError(t, err, "failed to create skipper collector: %v", err)
			collected, err := collector.GetMetrics(context.Background())
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

func TestSkipperCollector(t *testing.T) {
	for _, tc := range []struct {
		msg             string
		metric          int
		backend         string
		resourceName    string
		hostnames       []string
		expectedQuery   string
		collectedMetric int
		expectError     bool
		fakedAverage    bool
		namespace       string
		backendWeights  map[string]float64
		replicas        int32
		readyReplicas   int32
	}{
		{
			msg:             "test unweighted hpa",
			metric:          1000,
			resourceName:    "dummy-ingress",
			hostnames:       []string{"example.org"},
			expectedQuery:   `scalar(sum(rate(skipper_serve_host_duration_seconds_count{host=~"example_org"}[1m])) * 1.0000)`,
			collectedMetric: 1000,
			namespace:       "default",
			backend:         "dummy-backend",
			replicas:        1,
			readyReplicas:   1,
		},
		{
			msg:             "test weighted backend",
			metric:          1000,
			resourceName:    "dummy-ingress",
			hostnames:       []string{"example.org"},
			expectedQuery:   `scalar(sum(rate(skipper_serve_host_duration_seconds_count{host=~"example_org"}[1m])) * 0.4000)`,
			collectedMetric: 1000,
			namespace:       "default",
			backend:         "backend1",
			backendWeights:  map[string]float64{"backend2": 60.0, "backend1": 40},
			replicas:        1,
			readyReplicas:   1,
		},
		{
			msg:             "test multiple hostnames",
			metric:          1000,
			resourceName:    "dummy-ingress",
			hostnames:       []string{"example.org", "foo.bar.com", "test.org"},
			expectedQuery:   `scalar(sum(rate(skipper_serve_host_duration_seconds_count{host=~"example_org|foo_bar_com|test_org"}[1m])) * 0.4000)`,
			collectedMetric: 1000,
			namespace:       "default",
			backend:         "backend1",
			backendWeights:  map[string]float64{"backend2": 60, "backend1": 40},
			replicas:        1,
			readyReplicas:   1,
		},
		{
			msg:             "test multiple replicas",
			metric:          1000,
			resourceName:    "dummy-ingress",
			hostnames:       []string{"example.org"},
			expectedQuery:   `scalar(sum(rate(skipper_serve_host_duration_seconds_count{host=~"example_org"}[1m])) * 0.5000)`,
			collectedMetric: 200,
			fakedAverage:    true,
			namespace:       "default",
			backend:         "backend1",
			backendWeights:  map[string]float64{"backend2": 50, "backend1": 50},
			replicas:        5,
			readyReplicas:   5,
		},
		{
			msg:             "test multiple replicas not calculating average internally",
			metric:          1500,
			resourceName:    "dummy-ingress",
			hostnames:       []string{"example.org"},
			expectedQuery:   `scalar(sum(rate(skipper_serve_host_duration_seconds_count{host=~"example_org"}[1m])) * 0.5000)`,
			collectedMetric: 1500,
			namespace:       "default",
			backend:         "backend1",
			backendWeights:  map[string]float64{"backend2": 50, "backend1": 50},
			replicas:        5, // this is not taken into account
			readyReplicas:   5,
		},
		{
			msg:             "test zero weight backends",
			metric:          0,
			resourceName:    "dummy-ingress",
			hostnames:       []string{"example.org"},
			expectedQuery:   `scalar(sum(rate(skipper_serve_host_duration_seconds_count{host=~"example_org"}[1m])) * 0.0000)`,
			collectedMetric: 0,
			namespace:       "default",
			backend:         "backend1",
			backendWeights:  map[string]float64{"backend2": 100, "backend1": 0},
			replicas:        5,
			readyReplicas:   5,
		},
		{
			msg:             "test backend is not set",
			metric:          0,
			resourceName:    "dummy-ingress",
			hostnames:       []string{"example.org"},
			expectedQuery:   `scalar(sum(rate(skipper_serve_host_duration_seconds_count{host=~"example_org"}[1m])) * 0.0000)`,
			collectedMetric: 0,
			namespace:       "default",
			backend:         "backend3",
			backendWeights:  map[string]float64{"backend2": 100, "backend1": 0},
			replicas:        1,
			readyReplicas:   1,
		},
		{
			msg:             "test no annotations set",
			metric:          1500,
			resourceName:    "dummy-ingress",
			hostnames:       []string{"example.org"},
			expectedQuery:   `scalar(sum(rate(skipper_serve_host_duration_seconds_count{host=~"example_org"}[1m])) * 1.0000)`,
			collectedMetric: 1500,
			namespace:       "default",
			backend:         "backend3",
			backendWeights:  map[string]float64{},
			replicas:        1,
			readyReplicas:   1,
		},
		{
			msg:            "test annotations are set but backend is missing",
			metric:         1500,
			resourceName:   "dummy-ingress",
			hostnames:      []string{"example.org"},
			expectedQuery:  `scalar(sum(rate(skipper_serve_host_duration_seconds_count{host=~"example_org"}[1m])) * 1.0000)`,
			expectError:    true,
			namespace:      "default",
			backend:        "",
			backendWeights: map[string]float64{"backend2": 100, "backend1": 0},
			replicas:       1,
			readyReplicas:  1,
		},
		{
			msg:             "test annotations are missing and backend is unset",
			metric:          1500,
			resourceName:    "dummy-ingress",
			hostnames:       []string{"example.org"},
			expectedQuery:   `scalar(sum(rate(skipper_serve_host_duration_seconds_count{host=~"example_org"}[1m])) * 1.0000)`,
			collectedMetric: 1500,
			namespace:       "default",
			backend:         "",
			backendWeights:  nil,
			replicas:        1,
			readyReplicas:   1,
		},
	} {
		t.Run(tc.msg, func(t *testing.T) {
			client := fake.NewSimpleClientset()
			backendWeights := make(map[string]map[string]float64)
			if len(tc.backendWeights) > 0 {
				backendWeights = map[string]map[string]float64{testBackendWeightsAnnotation: tc.backendWeights}
			}
			err := makeIngress(client, tc.namespace, tc.resourceName, tc.backend, tc.hostnames, backendWeights)
			require.NoError(t, err)
			rgClient := rgfake.NewSimpleClientset()
			err = makeRoutegroup(rgClient, tc.namespace, tc.resourceName, tc.hostnames, tc.backendWeights)
			require.NoError(t, err)
			ingressHPA := makeIngressHPA(tc.namespace, tc.resourceName, tc.backend)
			rgHPA := makeRGHPA(tc.namespace, tc.resourceName, tc.backend)
			_, err = newDeployment(client, tc.namespace, tc.backend, tc.replicas, tc.readyReplicas)
			for _, hpa := range []*autoscalingv2.HorizontalPodAutoscaler{ingressHPA, rgHPA} {
				kind := hpa.Spec.Metrics[0].Object.DescribedObject.Kind
				plugin := makePlugin(tc.metric)
				config := makeConfig(tc.resourceName, tc.namespace, kind, tc.backend, tc.fakedAverage)
				require.NoError(t, err)
				collector, err := NewSkipperCollector(client, rgClient, plugin, hpa, config, time.Minute, []string{testBackendWeightsAnnotation}, tc.backend)
				require.NoError(t, err, "failed to create skipper collector: %v", err)
				collected, err := collector.GetMetrics(context.Background())
				if tc.expectError {
					require.Error(t, err, "%s", kind)
				} else {
					require.NoError(t, err, "%s", kind)
					require.Equal(t, map[string]string{"query": tc.expectedQuery}, plugin.config, "%s", kind)
					require.NoError(t, err, "%s: failed to collect metrics: %v", kind, err)
					require.Len(t, collected, 1, "%s: the number of metrics returned is not 1", kind)
					require.EqualValues(t, tc.collectedMetric, collected[0].Custom.Value.Value(), "%s: the returned metric is not expected value", kind)
				}
			}
		})
	}
}

func makeIngress(client kubernetes.Interface, namespace, resourceName, backend string, hostnames []string, backendWeights map[string]map[string]float64) error {
	annotations := make(map[string]string)
	for anno, weights := range backendWeights {
		sWeights, err := json.Marshal(weights)
		if err != nil {
			return err
		}
		annotations[anno] = string(sWeights)
	}
	ingress := &netv1.Ingress{
		ObjectMeta: metav1.ObjectMeta{
			Name:        resourceName,
			Annotations: annotations,
		},
		Spec: netv1.IngressSpec{
			DefaultBackend: &netv1.IngressBackend{
				Service: &netv1.IngressServiceBackend{
					Name: backend,
				},
			},
			TLS: nil,
		},
		Status: netv1.IngressStatus{
			LoadBalancer: netv1.IngressLoadBalancerStatus{
				Ingress: nil,
			},
		},
	}
	for _, hostname := range hostnames {
		ingress.Spec.Rules = append(ingress.Spec.Rules, netv1.IngressRule{
			Host: hostname,
		})
	}
	_, err := client.NetworkingV1().Ingresses(namespace).Create(context.TODO(), ingress, metav1.CreateOptions{})
	return err
}

func makeIngressHPA(namespace, name, backend string) *autoscalingv2.HorizontalPodAutoscaler {
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
						DescribedObject: autoscalingv2.CrossVersionObjectReference{Name: name, APIVersion: "extensions/v1", Kind: "Ingress"},
						Metric:          autoscalingv2.MetricIdentifier{Name: fmt.Sprintf("%s,%s", rpsMetricName, backend)},
					},
				},
			},
		},
	}
}

func makeRoutegroup(rgClient rginterface.Interface, namespace, resourceName string, hostnames []string, backendWeights map[string]float64) error {
	var backends []rgv1.RouteGroupBackendReference
	for backend, weight := range backendWeights {
		backends = append(backends, rgv1.RouteGroupBackendReference{BackendName: backend, Weight: int(weight)})
	}

	rg := &rgv1.RouteGroup{
		ObjectMeta: metav1.ObjectMeta{
			Name: resourceName,
		},
		Spec: rgv1.RouteGroupSpec{
			Hosts:           hostnames,
			DefaultBackends: backends,
		},
	}
	_, err := rgClient.ZalandoV1().RouteGroups(namespace).Create(context.TODO(), rg, metav1.CreateOptions{})
	return err
}

func makeRGHPA(namespace, name, backend string) *autoscalingv2.HorizontalPodAutoscaler {
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
						DescribedObject: autoscalingv2.CrossVersionObjectReference{Name: name, APIVersion: "zalando.org/v1", Kind: "RouteGroup"},
						Metric:          autoscalingv2.MetricIdentifier{Name: fmt.Sprintf("%s,%s", rpsMetricName, backend)},
					},
				},
			},
		},
	}
}

func makeConfig(resourceName, namespace, kind, backend string, fakedAverage bool) *MetricConfig {
	config := &MetricConfig{
		MetricTypeName: MetricTypeName{Metric: autoscalingv2.MetricIdentifier{Name: fmt.Sprintf("%s,%s", rpsMetricName, backend)}},
		ObjectReference: custom_metrics.ObjectReference{
			Name:      resourceName,
			Namespace: namespace,
			Kind:      kind,
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
