package provider

import (
	"github.com/davecgh/go-spew/spew"
	"github.com/kubernetes-incubator/custom-metrics-apiserver/pkg/provider"
	"github.com/stretchr/testify/require"
	"github.com/zalando-incubator/kube-metrics-adapter/pkg/collector"
	"k8s.io/api/autoscaling/v2beta1"
	"k8s.io/apimachinery/pkg/api/resource"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/metrics/pkg/apis/custom_metrics"
	"testing"
)

func TestMetricStore(t *testing.T) {
	var metricStoreTests = []struct {
		insert collector.CollectedMetric
		list   []provider.CustomMetricInfo
		byName struct {
			name types.NamespacedName
			info provider.CustomMetricInfo
		}
	}{
		{
			insert: collector.CollectedMetric{
				Type: v2beta1.MetricSourceType("Object"),
				Custom: custom_metrics.MetricValue{
					MetricName: "metric",
					Value:      *resource.NewQuantity(0, ""),
					DescribedObject: custom_metrics.ObjectReference{
						Name:      "metricObject",
						Namespace: "default",
					},
				},
			},
			list: []provider.CustomMetricInfo{
				{
					GroupResource: schema.GroupResource{},
					Namespaced:    true,
					Metric:        "metric",
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
					Metric:        "metric",
				},
			},
		},
	}

	metricsStore := NewMetricStore()

	// Insert a metric with value
	metricsStore.Insert(metricStoreTests[0].insert)

	// List a metric with value
	metricInfos := metricsStore.ListAllMetrics()
	require.Equal(t, metricStoreTests[0].list, metricInfos)

	// Get the metric by name
	metric := metricsStore.GetMetricsByName(metricStoreTests[0].byName.name, metricStoreTests[0].byName.info)

	require.Equal(t, metricStoreTests[0].insert.Custom, *metric)
	spew.Dump(metric)

}
