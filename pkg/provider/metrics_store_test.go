package provider

import (
	"testing"
	"time"

	"github.com/kubernetes-incubator/custom-metrics-apiserver/pkg/provider"
	"github.com/stretchr/testify/require"
	"github.com/zalando-incubator/kube-metrics-adapter/pkg/collector"
	autoscalingv2 "k8s.io/api/autoscaling/v2beta2"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/metrics/pkg/apis/custom_metrics"
	"k8s.io/metrics/pkg/apis/external_metrics"
)

func newMetricIdentifier(metricName string) custom_metrics.MetricIdentifier {
	selector := metav1.LabelSelector{}
	return custom_metrics.MetricIdentifier{
		Name:     metricName,
		Selector: &selector,
	}
}
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
				Type: autoscalingv2.MetricSourceType("Object"),
				Custom: custom_metrics.MetricValue{
					Metric: newMetricIdentifier("metric-per-unit"),
					Value:  *resource.NewQuantity(0, ""),
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
			test: "insert/list/get a Pod metric",
			insert: collector.CollectedMetric{
				Type: autoscalingv2.MetricSourceType("Object"),
				Custom: custom_metrics.MetricValue{
					Metric: newMetricIdentifier("metric-per-unit"),
					Value:  *resource.NewQuantity(0, ""),
					DescribedObject: custom_metrics.ObjectReference{
						Name:       "metricObject",
						Namespace:  "default",
						Kind:       "Pod",
						APIVersion: "core/v1",
					},
				},
			},
			list: []provider.CustomMetricInfo{
				{
					GroupResource: schema.GroupResource{
						Group:    "",
						Resource: "pods",
					},
					Namespaced: true,
					Metric:     "metric-per-unit",
				},
			},
			byName: struct {
				name types.NamespacedName
				info provider.CustomMetricInfo
			}{
				name: types.NamespacedName{Name: "metricObject", Namespace: "default"},
				info: provider.CustomMetricInfo{
					GroupResource: schema.GroupResource{
						Group:    "",
						Resource: "pods",
					},
					Namespaced: true,
					Metric:     "metric-per-unit",
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
					GroupResource: schema.GroupResource{
						Group:    "",
						Resource: "pods",
					},
					Namespaced: true,
					Metric:     "metric-per-unit",
				},
			},
		},
		{
			test: "insert/list/get a non-namespaced resource metric",
			insert: collector.CollectedMetric{
				Type: autoscalingv2.MetricSourceType("Object"),
				Custom: custom_metrics.MetricValue{
					Metric: newMetricIdentifier("metric-per-unit"),
					Value:  *resource.NewQuantity(0, ""),
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
		{
			test: "insert/list/get an Ingress metric",
			insert: collector.CollectedMetric{
				Type: autoscalingv2.MetricSourceType("Object"),
				Custom: custom_metrics.MetricValue{
					Metric: newMetricIdentifier("metric-per-unit"),
					Value:  *resource.NewQuantity(0, ""),
					DescribedObject: custom_metrics.ObjectReference{
						Name:       "metricObject",
						Kind:       "Ingress",
						APIVersion: "extensions/v1beta1",
					},
				},
			},
			list: []provider.CustomMetricInfo{
				{
					GroupResource: schema.GroupResource{
						Group:    "extensions",
						Resource: "ingresses",
					},
					Namespaced: false,
					Metric:     "metric-per-unit",
				},
			},
			byName: struct {
				name types.NamespacedName
				info provider.CustomMetricInfo
			}{
				name: types.NamespacedName{Name: "metricObject", Namespace: ""},
				info: provider.CustomMetricInfo{
					GroupResource: schema.GroupResource{
						Group:    "extensions",
						Resource: "ingresses",
					},
					Namespaced: false,
					Metric:     "metric-per-unit",
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
					GroupResource: schema.GroupResource{
						Group:    "extensions",
						Resource: "ingresses",
					},
					Namespaced: false,
					Metric:     "metric-per-unit",
				},
			},
		},
	}

	for _, tc := range metricStoreTests {
		t.Run(tc.test, func(t *testing.T) {
			metricsStore := NewMetricStore(func() time.Time {
				return time.Now().UTC().Add(15 * time.Minute)
			})

			// Insert a metric with value
			metricsStore.Insert(tc.insert)

			// List a metric with value
			metricInfos := metricsStore.ListAllMetrics()
			require.Equal(t, tc.list, metricInfos)

			// Get the metric by name
			metric := metricsStore.GetMetricsByName(tc.byName.name, tc.byName.info)

			if tc.insert.Custom != (custom_metrics.MetricValue{}) {
				require.Equal(t, tc.insert.Custom, *metric)
				metrics := metricsStore.GetMetricsBySelector(tc.byLabel.namespace, tc.byLabel.selector, tc.byLabel.info)
				require.Equal(t, tc.insert.Custom, metrics.Items[0])
			} else {
				require.Nil(t, metric)
				metrics := metricsStore.GetMetricsBySelector(tc.byLabel.namespace, tc.byLabel.selector, tc.byLabel.info)
				require.Nil(t, metrics)
			}

		})
	}

}

func TestMultipleMetricValues(t *testing.T) {
	var multiValueTests = []struct {
		test   string
		insert []collector.CollectedMetric
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
			test: "insert/list/get multiple Ingress metrics",
			insert: []collector.CollectedMetric{
				{
					Type: autoscalingv2.MetricSourceType("Object"),
					Custom: custom_metrics.MetricValue{
						Metric: newMetricIdentifier("metric-per-unit"),
						Value:  *resource.NewQuantity(0, ""),
						DescribedObject: custom_metrics.ObjectReference{
							Name:       "metricObject",
							Kind:       "Ingress",
							APIVersion: "extensions/v1beta1",
						},
					},
				},
				{
					Type: autoscalingv2.MetricSourceType("Object"),
					Custom: custom_metrics.MetricValue{
						Metric: newMetricIdentifier("metric-per-unit"),
						Value:  *resource.NewQuantity(1, ""),
						DescribedObject: custom_metrics.ObjectReference{
							Name:       "metricObject",
							Kind:       "Ingress",
							APIVersion: "extensions/v1beta1",
						},
					},
				},
			},
			list: []provider.CustomMetricInfo{
				{
					GroupResource: schema.GroupResource{
						Group:    "extensions",
						Resource: "ingresses",
					},
					Namespaced: false,
					Metric:     "metric-per-unit",
				},
			},
			byName: struct {
				name types.NamespacedName
				info provider.CustomMetricInfo
			}{
				name: types.NamespacedName{Name: "metricObject", Namespace: ""},
				info: provider.CustomMetricInfo{
					GroupResource: schema.GroupResource{
						Group:    "extensions",
						Resource: "ingresses",
					},
					Namespaced: false,
					Metric:     "metric-per-unit",
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
					GroupResource: schema.GroupResource{
						Group:    "extensions",
						Resource: "ingresses",
					},
					Namespaced: false,
					Metric:     "metric-per-unit",
				},
			},
		},
		{
			test: "insert/list/get multiple namespaced resource metrics",
			insert: []collector.CollectedMetric{
				{
					Type: autoscalingv2.MetricSourceType("Object"),
					Custom: custom_metrics.MetricValue{
						Metric: newMetricIdentifier("metric-per-unit"),
						Value:  *resource.NewQuantity(0, ""),
						DescribedObject: custom_metrics.ObjectReference{
							Name:       "metricObject",
							Namespace:  "default",
							Kind:       "Deployment",
							APIVersion: "apps/v1",
						},
					},
				},
				{
					Type: autoscalingv2.MetricSourceType("Object"),
					Custom: custom_metrics.MetricValue{
						Metric: newMetricIdentifier("metric-per-unit"),
						Value:  *resource.NewQuantity(1, ""),
						DescribedObject: custom_metrics.ObjectReference{
							Name:       "metricObject",
							Namespace:  "default",
							Kind:       "Deployment",
							APIVersion: "apps/v1",
						},
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
	}

	for _, tc := range multiValueTests {
		t.Run(tc.test, func(t *testing.T) {
			metricsStore := NewMetricStore(func() time.Time {
				return time.Now().UTC().Add(15 * time.Minute)
			})

			// Insert a metric with value
			for _, insert := range tc.insert {
				metricsStore.Insert(insert)

				// Get the metric by name
				metric := metricsStore.GetMetricsByName(tc.byName.name, tc.byName.info)
				require.Equal(t, insert.Custom, *metric)

				// Get the metric by label
				metrics := metricsStore.GetMetricsBySelector(tc.byLabel.namespace, tc.byLabel.selector, tc.byLabel.info)
				require.Equal(t, insert.Custom, metrics.Items[0])
			}

			// List a metric with value
			metricInfos := metricsStore.ListAllMetrics()
			require.Equal(t, tc.list, metricInfos)

		})
	}
}

func TestCustomMetricsStorageErrors(t *testing.T) {
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
			test:   "insert/list/get an empty metric",
			insert: collector.CollectedMetric{},
			list:   []provider.CustomMetricInfo{},
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
			test: "test that not all Kinds are mapped to a group/resource",
			insert: collector.CollectedMetric{
				Type: autoscalingv2.MetricSourceType("Object"),
				Custom: custom_metrics.MetricValue{
					Metric: newMetricIdentifier("metric-per-unit"),
					Value:  *resource.NewQuantity(0, ""),
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
					GroupResource: schema.GroupResource{
						Group:    "apps",
						Resource: "deployments",
					},
					Namespaced: true,
					Metric:     "metric-per-unit",
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
					GroupResource: schema.GroupResource{
						Group:    "apps",
						Resource: "deployments",
					},
					Namespaced: true,
					Metric:     "metric-per-unit",
				},
			},
		},
	}

	for _, tc := range metricStoreTests {
		t.Run(tc.test, func(t *testing.T) {
			metricsStore := NewMetricStore(func() time.Time {
				return time.Now().UTC().Add(15 * time.Minute)
			})

			// Insert a metric with value
			metricsStore.Insert(tc.insert)

			// List a metric with value
			metricInfos := metricsStore.ListAllMetrics()
			require.Equal(t, tc.list, metricInfos)

			// Get the metric by name
			metric := metricsStore.GetMetricsByName(tc.byName.name, tc.byName.info)
			require.Nil(t, metric)

			metrics := metricsStore.GetMetricsBySelector(tc.byLabel.namespace, tc.byLabel.selector, tc.byLabel.info)
			require.Equal(t, &custom_metrics.MetricValueList{}, metrics)

		})
	}
	var multiValueTests = []struct {
		test   string
		insert []collector.CollectedMetric
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
			test: "insert/list/get multiple metrics in different groups",
			insert: []collector.CollectedMetric{
				{
					Type: autoscalingv2.MetricSourceType("Object"),
					Custom: custom_metrics.MetricValue{
						Metric: newMetricIdentifier("metric-per-unit"),
						Value:  *resource.NewQuantity(0, ""),
						DescribedObject: custom_metrics.ObjectReference{
							Name:       "metricObject",
							Namespace:  "default",
							Kind:       "Ingress",
							APIVersion: "extensions/vbeta1",
						},
					},
				},
				{
					Type: autoscalingv2.MetricSourceType("Object"),
					Custom: custom_metrics.MetricValue{
						Metric: newMetricIdentifier("metric-per-unit"),
						Value:  *resource.NewQuantity(1, ""),
						DescribedObject: custom_metrics.ObjectReference{
							Name:       "metricObject",
							Namespace:  "default",
							Kind:       "Pod",
							APIVersion: "core/v1",
						},
					},
				},
				{
					Type: autoscalingv2.MetricSourceType("Object"),
					Custom: custom_metrics.MetricValue{
						Metric: newMetricIdentifier("metric-per-unit"),
						Value:  *resource.NewQuantity(1, ""),
						DescribedObject: custom_metrics.ObjectReference{
							Name:       "metricObject",
							Namespace:  "new-namespace",
							Kind:       "Pod",
							APIVersion: "core/v1",
						},
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
	}

	for _, tc := range multiValueTests {
		t.Run(tc.test, func(t *testing.T) {
			metricsStore := NewMetricStore(func() time.Time {
				return time.Now().UTC().Add(15 * time.Minute)
			})

			// Insert a metric with value
			for _, insert := range tc.insert {
				metricsStore.Insert(insert)
			}

		})
	}

}

func TestExternalMetricStorage(t *testing.T) {
	var metricStoreTests = []struct {
		test   string
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
				Type: autoscalingv2.MetricSourceType("External"),
				External: external_metrics.ExternalMetricValue{
					MetricName:   "metric-per-unit",
					Value:        *resource.NewQuantity(0, ""),
					MetricLabels: map[string]string{"application": "some-app"},
				},
			},
			list: provider.ExternalMetricInfo{
				Metric: "metric-per-unit",
			},
			get: struct {
				namespace string
				selector  labels.Selector
				info      provider.ExternalMetricInfo
			}{namespace: "",
				selector: labels.Everything(),
				info: provider.ExternalMetricInfo{
					Metric: "metric-per-unit",
				}},
		},
	}

	for _, tc := range metricStoreTests {
		t.Run(tc.test, func(t *testing.T) {
			metricsStore := NewMetricStore(func() time.Time {
				return time.Now().UTC().Add(15 * time.Minute)
			})

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

func TestMultipleExternalMetricStorage(t *testing.T) {
	var metricStoreTests = []struct {
		test   string
		insert []collector.CollectedMetric
		list   provider.ExternalMetricInfo
		get    struct {
			namespace string
			selector  labels.Selector
			info      provider.ExternalMetricInfo
		}
	}{
		{
			test: "the latest value overrides the last one",
			insert: []collector.CollectedMetric{
				{
					Type: autoscalingv2.MetricSourceType("External"),
					External: external_metrics.ExternalMetricValue{
						MetricName:   "metric-per-unit",
						Value:        *resource.NewQuantity(0, ""),
						MetricLabels: map[string]string{"application": "some-app"},
					},
				},
				{
					Type: autoscalingv2.MetricSourceType("External"),
					External: external_metrics.ExternalMetricValue{
						MetricName:   "metric-per-unit",
						Value:        *resource.NewQuantity(1, ""),
						MetricLabels: map[string]string{"application": "some-app"},
					},
				},
			},
			list: provider.ExternalMetricInfo{
				Metric: "metric-per-unit",
			},
			get: struct {
				namespace string
				selector  labels.Selector
				info      provider.ExternalMetricInfo
			}{namespace: "",
				selector: labels.Everything(),
				info: provider.ExternalMetricInfo{
					Metric: "metric-per-unit",
				}},
		},
	}

	for _, tc := range metricStoreTests {
		t.Run(tc.test, func(t *testing.T) {
			metricsStore := NewMetricStore(func() time.Time {
				return time.Now().UTC().Add(15 * time.Minute)
			})

			for _, insert := range tc.insert {
				// Insert a metric with value
				metricsStore.Insert(insert)

			}

			// Get the metric by name
			metrics, err := metricsStore.GetExternalMetric(tc.get.namespace, tc.get.selector, tc.get.info)
			require.NoError(t, err)
			require.NotContains(t, metrics.Items, tc.insert[0].External)
			require.Contains(t, metrics.Items, tc.insert[1].External)

			// List a metric with value
			metricInfos := metricsStore.ListAllExternalMetrics()
			require.Equal(t, tc.list, metricInfos[0])
		})
	}

}

func TestMetricsExpiration(t *testing.T) {
	// Temporarily Override global TTL to test expiration
	metricStore := NewMetricStore(func() time.Time {
		return time.Now().UTC().Add(time.Hour * -1)
	})

	customMetric := collector.CollectedMetric{
		Type: autoscalingv2.MetricSourceType("Object"),
		Custom: custom_metrics.MetricValue{
			Metric: newMetricIdentifier("metric-per-unit"),
			Value:  *resource.NewQuantity(0, ""),
			DescribedObject: custom_metrics.ObjectReference{
				Name:       "metricObject",
				Kind:       "Node",
				APIVersion: "core/v1",
			},
		},
	}

	externalMetric := collector.CollectedMetric{
		Type: autoscalingv2.MetricSourceType("External"),
		External: external_metrics.ExternalMetricValue{
			MetricName: "metric-per-unit",
			Value:      *resource.NewQuantity(0, ""),
		},
	}

	metricStore.Insert(customMetric)
	metricStore.Insert(externalMetric)

	metricStore.RemoveExpired()

	customMetricInfos := metricStore.ListAllMetrics()
	require.Len(t, customMetricInfos, 0)

	externalMetricInfos := metricStore.ListAllExternalMetrics()
	require.Len(t, externalMetricInfos, 0)

}

func TestMetricsNonExpiration(t *testing.T) {
	metricStore := NewMetricStore(func() time.Time {
		return time.Now().UTC().Add(15 * time.Minute)
	})

	customMetric := collector.CollectedMetric{
		Type: autoscalingv2.MetricSourceType("Object"),
		Custom: custom_metrics.MetricValue{
			Metric: newMetricIdentifier("metric-per-unit"),
			Value:  *resource.NewQuantity(0, ""),
			DescribedObject: custom_metrics.ObjectReference{
				Name:       "metricObject",
				Kind:       "Node",
				APIVersion: "core/v1",
			},
		},
	}

	externalMetric := collector.CollectedMetric{
		Type: autoscalingv2.MetricSourceType("External"),
		External: external_metrics.ExternalMetricValue{
			MetricName: "metric-per-unit",
			Value:      *resource.NewQuantity(0, ""),
		},
	}

	metricStore.Insert(customMetric)
	metricStore.Insert(externalMetric)

	metricStore.RemoveExpired()

	customMetricInfos := metricStore.ListAllMetrics()
	require.Len(t, customMetricInfos, 1)

	externalMetricInfos := metricStore.ListAllExternalMetrics()
	require.Len(t, externalMetricInfos, 1)

}
