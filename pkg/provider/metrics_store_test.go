package provider

import (
	"github.com/kubernetes-incubator/custom-metrics-apiserver/pkg/provider"
	"github.com/stretchr/testify/require"
	"github.com/zalando-incubator/kube-metrics-adapter/pkg/collector"
	"k8s.io/api/autoscaling/v2beta1"
	"k8s.io/apimachinery/pkg/api/resource"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/metrics/pkg/apis/custom_metrics"
	"k8s.io/metrics/pkg/apis/external_metrics"
	"testing"
)

func TestInternalMetricStorage(t *testing.T) {
	var metricStoreTests = []struct {
		test   string
		insert collector.CollectedMetric
		list   []provider.CustomMetricInfo
		byName struct {
			name types.NamespacedName
			info provider.CustomMetricInfo
		}
		byLabel struct {
			namespace string
			selector  labels.Selector
			info      provider.CustomMetricInfo
		}
	}{
		{
			test: "insert/list/get a namespaced resource metric",
			insert: collector.CollectedMetric{
				Type: v2beta1.MetricSourceType("Object"),
				Custom: custom_metrics.MetricValue{
					MetricName: "metric-per-unit",
					Value:      *resource.NewQuantity(0, ""),
					DescribedObject: custom_metrics.ObjectReference{
						Name:       "metricObject",
						Namespace:  "default",
						Kind:       "Deployment",
						APIVersion: "apps/v1",
					},
				},
			},
			list: []provider.CustomMetricInfo{
				{
					GroupResource: schema.GroupResource{},
					Namespaced:    true,
					Metric:        "metric-per-unit",
				},
			},
			byName: struct {
				name types.NamespacedName
				info provider.CustomMetricInfo
			}{
				name: types.NamespacedName{Name: "metricObject", Namespace: "default"},
				info: provider.CustomMetricInfo{
					GroupResource: schema.GroupResource{},
					Namespaced:    true,
					Metric:        "metric-per-unit",
				},
			},
			byLabel: struct {
				namespace string
				selector  labels.Selector
				info      provider.CustomMetricInfo
			}{
				namespace: "default",
				selector:  labels.Everything(),
				info: provider.CustomMetricInfo{
					GroupResource: schema.GroupResource{},
					Namespaced:    true,
					Metric:        "metric-per-unit",
				},
			},
		},
		{
			test: "insert/list/get a non-namespaced resource metric",
			insert: collector.CollectedMetric{
				Type: v2beta1.MetricSourceType("Object"),
				Custom: custom_metrics.MetricValue{
					MetricName: "metric-per-unit",
					Value:      *resource.NewQuantity(0, ""),
					DescribedObject: custom_metrics.ObjectReference{
						Name:       "metricObject",
						Kind:       "Node",
						APIVersion: "core/v1",
					},
				},
			},
			list: []provider.CustomMetricInfo{
				{
					GroupResource: schema.GroupResource{},
					Namespaced:    false,
					Metric:        "metric-per-unit",
				},
			},
			byName: struct {
				name types.NamespacedName
				info provider.CustomMetricInfo
			}{
				name: types.NamespacedName{Name: "metricObject", Namespace: ""},
				info: provider.CustomMetricInfo{
					GroupResource: schema.GroupResource{},
					Namespaced:    false,
					Metric:        "metric-per-unit",
				},
			},
			byLabel: struct {
				namespace string
				selector  labels.Selector
				info      provider.CustomMetricInfo
			}{
				namespace: "",
				selector:  labels.Everything(),
				info: provider.CustomMetricInfo{
					GroupResource: schema.GroupResource{},
					Namespaced:    false,
					Metric:        "metric-per-unit",
				},
			},
		},
	}

	for _, tc := range metricStoreTests {
		t.Run(tc.test, func(t *testing.T) {
			metricsStore := NewMetricStore()

			// Insert a metric with value
			metricsStore.Insert(tc.insert)

			// List a metric with value
			metricInfos := metricsStore.ListAllMetrics()
			require.Equal(t, tc.list, metricInfos)

			// Get the metric by name
			metric := metricsStore.GetMetricsByName(tc.byName.name, tc.byName.info)

			require.Equal(t, tc.insert.Custom, *metric)
			metrics := metricsStore.GetMetricsBySelector(tc.byLabel.namespace, tc.byLabel.selector, tc.byLabel.info)
			require.Equal(t, tc.insert.Custom, metrics.Items[0])

		})
	}

}

func TestExternalMetricStorage(t *testing.T) {
	var metricStoreTests = []struct {
		test string
		insert collector.CollectedMetric
		list   provider.ExternalMetricInfo
		get    struct {
			namespace string
			selector  labels.Selector
			info      provider.ExternalMetricInfo
		}
	}{
		{
			test: "insert/list/get an external metric",
			insert: collector.CollectedMetric{
				Type: v2beta1.MetricSourceType("External"),
				External: external_metrics.ExternalMetricValue{
					MetricName: "metric-per-unit",
					Value:      *resource.NewQuantity(0, ""),
				},
			},
			list: provider.ExternalMetricInfo{
				Metric: "metric-per-unit",
			},
			get: struct {
				namespace string
				selector  labels.Selector
				info      provider.ExternalMetricInfo
			}{	namespace: "",
				selector: labels.Everything(),
				info: provider.ExternalMetricInfo{
				Metric: "metric-per-unit",
			}},
		},
	}

	for _, tc := range metricStoreTests {
		t.Run(tc.test, func(t *testing.T) {
			metricsStore := NewMetricStore()

			// Insert a metric with value
			metricsStore.Insert(tc.insert)

			// List a metric with value
			metricInfos := metricsStore.ListAllExternalMetrics()
			require.Equal(t, tc.list, metricInfos[0])

			// Get the metric by name
			metrics, err := metricsStore.GetExternalMetric(tc.get.namespace, tc.get.selector, tc.get.info)
			require.NoError(t, err)
			require.Equal(t, tc.insert.External, metrics.Items[0])

		})
	}

}
