package collector

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	appsv1 "k8s.io/api/apps/v1"
	autoscalingv2 "k8s.io/api/autoscaling/v2beta2"
	corev1 "k8s.io/api/core/v1"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/kubernetes/fake"
)

const (
	testNamespace         = "test-namespace"
	applicationLabelName  = "application"
	applicationLabelValue = "test-application"
	testDeploymentName    = "test-application"
	testInterval          = 10 * time.Second
)

func TestPodCollector(t *testing.T) {
	for _, tc := range []struct {
		name    string
		metrics [][]int64
		result  []int64
	}{
		{
			name:    "simple",
			metrics: [][]int64{{1}, {3}, {8}, {5}, {2}},
			result:  []int64{1, 3, 8, 5, 2},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			client := fake.NewSimpleClientset()
			plugin := NewPodCollectorPlugin(client)
			makeTestDeployment(t, client)
			host, port, metricsHandler := makeTestHTTPServer(t, tc.metrics)
			lastReadyTransitionTimeTimestamp := v1.NewTime(time.Now().Add(time.Duration(-30) * time.Second))
			minPodReadyAge := time.Duration(0 * time.Second)
			podCondition := corev1.PodCondition{Type: corev1.PodReady, Status: corev1.ConditionTrue, LastTransitionTime: lastReadyTransitionTimeTimestamp}
			podDeletionTimestamp := time.Time{}
			makeTestPods(t, host, port, "test-metric", client, 5, podCondition,podDeletionTimestamp)
			testHPA := makeTestHPA(t, client)
			testConfig := makeTestConfig(port, minPodReadyAge)
			collector, err := plugin.NewCollector(testHPA, testConfig, testInterval)
			require.NoError(t, err)
			metrics, err := collector.GetMetrics()
			require.NoError(t, err)
			require.Equal(t, len(metrics), int(metricsHandler.calledCounter))
			var values []int64
			for _, m := range metrics {
				values = append(values, m.Custom.Value.Value())
			}
			require.ElementsMatch(t, tc.result, values)
		})
	}
}

func TestPodCollectorWithMinPodReadyAge(t *testing.T) {
	for _, tc := range []struct {
		name    string
		metrics [][]int64
		result  []int64
	}{
		{
			name:    "simple-with-min-pod-ready-age",
			metrics: [][]int64{{1}, {3}, {8}, {5}, {2}},
			result:  []int64{},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			client := fake.NewSimpleClientset()
			plugin := NewPodCollectorPlugin(client)
			makeTestDeployment(t, client)
			host, port, metricsHandler := makeTestHTTPServer(t, tc.metrics)
			// Setting pods age to 30 seconds
			lastReadyTransitionTimeTimestamp := v1.NewTime(time.Now().Add(time.Duration(-30) * time.Second))
			// Pods that are not older that 60 seconds (all in this case) should not be processed
			minPodReadyAge := time.Duration(60 * time.Second)
			podCondition := corev1.PodCondition{Type: corev1.PodReady, Status: corev1.ConditionTrue, LastTransitionTime: lastReadyTransitionTimeTimestamp}
			podDeletionTimestamp := time.Time{}
			makeTestPods(t, host, port, "test-metric", client, 5, podCondition, podDeletionTimestamp)
			testHPA := makeTestHPA(t, client)
			testConfig := makeTestConfig(port, minPodReadyAge)
			collector, err := plugin.NewCollector(testHPA, testConfig, testInterval)
			require.NoError(t, err)
			metrics, err := collector.GetMetrics()
			require.NoError(t, err)
			require.Equal(t, len(metrics), int(metricsHandler.calledCounter))
			var values []int64
			for _, m := range metrics {
				values = append(values, m.Custom.Value.Value())
			}
			require.ElementsMatch(t, tc.result, values)
		})
	}
}

func TestPodCollectorWithPodCondition(t *testing.T) {
	for _, tc := range []struct {
		name    string
		metrics [][]int64
		result  []int64
	}{
		{
			name:    "simple-with-pod-condition",
			metrics: [][]int64{{1}, {3}, {8}, {5}, {2}},
			result:  []int64{},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			client := fake.NewSimpleClientset()
			plugin := NewPodCollectorPlugin(client)
			makeTestDeployment(t, client)
			host, port, metricsHandler := makeTestHTTPServer(t, tc.metrics)
			lastScheduledTransitionTimeTimestamp := v1.NewTime(time.Now().Add(time.Duration(-30) * time.Second))
			minPodReadyAge := time.Duration(0 * time.Second)
			podDeletionTimestamp := time.Time{}
			//Pods in state corev1.PodReady == corev1.ConditionFalse should not be processed
			podCondition := corev1.PodCondition{Type: corev1.PodReady, Status: corev1.ConditionFalse, LastTransitionTime: lastScheduledTransitionTimeTimestamp}
			makeTestPods(t, host, port, "test-metric", client, 5, podCondition, podDeletionTimestamp)
			testHPA := makeTestHPA(t, client)
			testConfig := makeTestConfig(port, minPodReadyAge)
			collector, err := plugin.NewCollector(testHPA, testConfig, testInterval)
			require.NoError(t, err)
			metrics, err := collector.GetMetrics()
			require.NoError(t, err)
			require.Equal(t, len(metrics), int(metricsHandler.calledCounter))
			var values []int64
			for _, m := range metrics {
				values = append(values, m.Custom.Value.Value())
			}
			require.ElementsMatch(t, tc.result, values)
		})
	}
}


func TestPodCollectorWithPodTerminatingCondition(t *testing.T) {
	for _, tc := range []struct {
		name    string
		metrics [][]int64
		result  []int64
	}{
		{
			name:    "simple-with-pod-condition",
			metrics: [][]int64{{1}, {3}, {8}, {5}, {2}},
			result:  []int64{},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			client := fake.NewSimpleClientset()
			plugin := NewPodCollectorPlugin(client)
			makeTestDeployment(t, client)
			host, port, metricsHandler := makeTestHTTPServer(t, tc.metrics)
			lastScheduledTransitionTimeTimestamp := v1.NewTime(time.Now().Add(time.Duration(-30) * time.Second))
			minPodReadyAge := time.Duration(0 * time.Second)
			//Pods with podDeletionTimestamp should not be processed
			podDeletionTimestamp := time.Now()
			podCondition := corev1.PodCondition{Type: corev1.PodReady, Status: corev1.ConditionTrue, LastTransitionTime: lastScheduledTransitionTimeTimestamp}
			makeTestPods(t, host, port, "test-metric", client, 5, podCondition, podDeletionTimestamp)
			testHPA := makeTestHPA(t, client)
			testConfig := makeTestConfig(port, minPodReadyAge)
			collector, err := plugin.NewCollector(testHPA, testConfig, testInterval)
			require.NoError(t, err)
			metrics, err := collector.GetMetrics()
			require.NoError(t, err)
			require.Equal(t, len(metrics), int(metricsHandler.calledCounter))
			var values []int64
			for _, m := range metrics {
				values = append(values, m.Custom.Value.Value())
			}
			require.ElementsMatch(t, tc.result, values)
		})
	}
}

type testMetricResponse struct {
	Values []int64 `json:"values"`
}
type testMetricsHandler struct {
	values        [][]int64
	calledCounter uint
	t             *testing.T
	metricsPath   string
	sync.RWMutex
}

func (h *testMetricsHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	h.Lock()
	defer h.Unlock()

	require.Equal(h.t, h.metricsPath, r.URL.Path)
	require.Less(h.t, int(h.calledCounter), len(h.values))
	response, err := json.Marshal(testMetricResponse{Values: h.values[h.calledCounter]})
	require.NoError(h.t, err)
	w.Header().Set("Content-Type", "application/json")
	_, err = w.Write(response)
	require.NoError(h.t, err)
	h.calledCounter++
}

func makeTestHTTPServer(t *testing.T, values [][]int64) (string, string, *testMetricsHandler) {
	metricsHandler := &testMetricsHandler{values: values, t: t, metricsPath: "/metrics"}
	server := httptest.NewServer(metricsHandler)
	url, err := url.Parse(server.URL)
	require.NoError(t, err)
	return url.Hostname(), url.Port(), metricsHandler
}

func makeTestConfig(port string, minPodReadyAge time.Duration) *MetricConfig {
	return &MetricConfig{
		CollectorType:  "json-path",
		Config:         map[string]string{"json-key": "$.values", "port": port, "path": "/metrics", "aggregator": "sum"},
		MinPodReadyAge: minPodReadyAge,
	}
}

func makeTestPods(t *testing.T, testServer string, metricName string, port string, client kubernetes.Interface, replicas int, podCondition corev1.PodCondition, podDeletionTimestamp time.Time) {


	for i := 0; i < replicas; i++ {
		testPod := &corev1.Pod{
			ObjectMeta: v1.ObjectMeta{
				Name:   fmt.Sprintf("test-pod-%d", i),
				Labels: map[string]string{applicationLabelName: applicationLabelValue},
				Annotations: map[string]string{
					fmt.Sprintf("metric-config.pods.%s.json-path/port", metricName): port,
				},
			},
			Status: corev1.PodStatus{
				PodIP:      testServer,
				Conditions: []corev1.PodCondition{podCondition},
			},
		}
    
		if podDeletionTimestamp.IsZero(){
			testPod.ObjectMeta.DeletionTimestamp = nil
		} else {
			testPod.ObjectMeta.DeletionTimestamp = &v1.Time{Time: podDeletionTimestamp}
		}
		_, err := client.CoreV1().Pods(testNamespace).Create(context.TODO(), testPod, v1.CreateOptions{})
		require.NoError(t, err)
	}
}

func makeTestDeployment(t *testing.T, client kubernetes.Interface) *appsv1.Deployment {
	deployment := appsv1.Deployment{
		ObjectMeta: v1.ObjectMeta{Name: testDeploymentName},
		Spec: appsv1.DeploymentSpec{
			Selector: &v1.LabelSelector{
				MatchLabels: map[string]string{applicationLabelName: applicationLabelValue},
			},
		},
	}
	_, err := client.AppsV1().Deployments(testNamespace).Create(context.TODO(), &deployment, v1.CreateOptions{})
	require.NoError(t, err)
	return &deployment

}

func makeTestHPA(t *testing.T, client kubernetes.Interface) *autoscalingv2.HorizontalPodAutoscaler {
	hpa := &autoscalingv2.HorizontalPodAutoscaler{
		ObjectMeta: v1.ObjectMeta{
			Name:      "test-hpa",
			Namespace: testNamespace,
		},
		Spec: autoscalingv2.HorizontalPodAutoscalerSpec{
			ScaleTargetRef: autoscalingv2.CrossVersionObjectReference{
				Kind:       "Deployment",
				Name:       testDeploymentName,
				APIVersion: "apps/v1",
			},
		},
	}
	_, err := client.AutoscalingV2beta2().HorizontalPodAutoscalers("test-namespace").Create(context.TODO(), hpa, v1.CreateOptions{})
	require.NoError(t, err)
	return hpa
}
