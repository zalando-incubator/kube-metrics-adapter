package provider

import (
	"fmt"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/kubernetes-incubator/custom-metrics-apiserver/pkg/provider"
	"github.com/zalando-incubator/kube-metrics-adapter/pkg/collector"
	autoscalingv2 "k8s.io/api/autoscaling/v2beta2"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/metrics/pkg/apis/custom_metrics"
	"k8s.io/metrics/pkg/apis/external_metrics"
)

// customMetricsStoredMetric is a wrapper around custom_metrics.MetricValue with a metricsTTL used
// to clean up stale metrics from the customMetricsStore.
type customMetricsStoredMetric struct {
	Value custom_metrics.MetricValue
	TTL   time.Time
}

type externalMetricsStoredMetric struct {
	Value external_metrics.ExternalMetricValue
	TTL   time.Time
}

// MetricStore is a simple in-memory Metrics Store for HPA metrics.
type MetricStore struct {
	customMetricsStore   map[string]map[schema.GroupResource]map[string]map[string]customMetricsStoredMetric
	externalMetricsStore map[string]map[string]externalMetricsStoredMetric
	metricsTTLCalculator func() time.Time
	sync.RWMutex
}

// NewMetricStore initializes an empty Metrics Store.
func NewMetricStore(ttlCalculator func() time.Time) *MetricStore {
	return &MetricStore{
		customMetricsStore:   make(map[string]map[schema.GroupResource]map[string]map[string]customMetricsStoredMetric, 0),
		externalMetricsStore: make(map[string]map[string]externalMetricsStoredMetric, 0),
		metricsTTLCalculator: ttlCalculator,
	}
}

// Insert inserts a collected metric into the metric customMetricsStore.
func (s *MetricStore) Insert(value collector.CollectedMetric) {
	switch value.Type {
	case autoscalingv2.ObjectMetricSourceType, autoscalingv2.PodsMetricSourceType:
		s.insertCustomMetric(value.Custom)
	case autoscalingv2.ExternalMetricSourceType:
		s.insertExternalMetric(value.External)
	}
}

// insertCustomMetric inserts a custom metric plus labels into the store.
func (s *MetricStore) insertCustomMetric(value custom_metrics.MetricValue) {
	s.Lock()
	defer s.Unlock()

	// TODO: handle this mapping nicer
	var groupResource schema.GroupResource
	switch value.DescribedObject.Kind {
	case "Pod":
		groupResource = schema.GroupResource{
			Resource: "pods",
		}
	case "Ingress":
		// group can be either `extentions` or `networking.k8s.io`
		group := "extensions"
		gv, err := schema.ParseGroupVersion(value.DescribedObject.APIVersion)
		if err == nil {
			group = gv.Group
		}
		groupResource = schema.GroupResource{
			Resource: "ingresses",
			Group:    group,
		}
	}

	metric := customMetricsStoredMetric{
		Value: value,
		TTL:   s.metricsTTLCalculator(), // TODO: make TTL configurable
	}

	metrics, ok := s.customMetricsStore[value.Metric.Name]
	if !ok {
		s.customMetricsStore[value.Metric.Name] = map[schema.GroupResource]map[string]map[string]customMetricsStoredMetric{
			groupResource: {
				value.DescribedObject.Namespace: map[string]customMetricsStoredMetric{
					value.DescribedObject.Name: metric,
				},
			},
		}
		return
	}

	group, ok := metrics[groupResource]
	if !ok {
		metrics[groupResource] = map[string]map[string]customMetricsStoredMetric{
			value.DescribedObject.Namespace: {
				value.DescribedObject.Name: metric,
			},
		}
		return
	}

	namespace, ok := group[value.DescribedObject.Namespace]
	if !ok {
		group[value.DescribedObject.Namespace] = map[string]customMetricsStoredMetric{
			value.DescribedObject.Name: metric,
		}
		return
	}

	namespace[value.DescribedObject.Name] = metric
}

// insertExternalMetric inserts an external metric into the store.
func (s *MetricStore) insertExternalMetric(metric external_metrics.ExternalMetricValue) {
	s.Lock()
	defer s.Unlock()

	storedMetric := externalMetricsStoredMetric{
		Value: metric,
		TTL:   s.metricsTTLCalculator(), // TODO: make TTL configurable
	}

	labelsKey := hashLabelMap(metric.MetricLabels)

	if metrics, ok := s.externalMetricsStore[metric.MetricName]; ok {
		metrics[labelsKey] = storedMetric
	} else {
		s.externalMetricsStore[metric.MetricName] = map[string]externalMetricsStoredMetric{
			labelsKey: storedMetric,
		}
	}
}

// hashLabelMap converts a map into a sorted string to provide a stable
// representation of a labels map.
func hashLabelMap(labels map[string]string) string {
	strLabels := make([]string, 0, len(labels))
	for k, v := range labels {
		strLabels = append(strLabels, fmt.Sprintf("%s=%s", k, v))
	}
	sort.Strings(strLabels)
	return strings.Join(strLabels, ",")
}

// GetMetricsBySelector gets metric from the customMetricsStore using a label selector to
// find metrics for matching resources.
func (s *MetricStore) GetMetricsBySelector(namespace string, selector labels.Selector, info provider.CustomMetricInfo) *custom_metrics.MetricValueList {
	matchedMetrics := make([]custom_metrics.MetricValue, 0)

	s.RLock()
	defer s.RUnlock()

	metrics, ok := s.customMetricsStore[info.Metric]
	if !ok {
		return &custom_metrics.MetricValueList{}
	}

	group, ok := metrics[info.GroupResource]
	if !ok {
		return &custom_metrics.MetricValueList{}
	}

	if !info.Namespaced {
		for _, metricMap := range group {
			for _, metric := range metricMap {
				if selector.Matches(labels.Set(metric.Value.Metric.Selector.MatchLabels)) {
					matchedMetrics = append(matchedMetrics, metric.Value)
				}
			}
		}
	} else if metricMap, ok := group[namespace]; ok {
		for _, metric := range metricMap {
			if metric.Value.Metric.Selector != nil && selector.Matches(labels.Set(metric.Value.Metric.Selector.MatchLabels)) {
				matchedMetrics = append(matchedMetrics, metric.Value)
			}
		}
	}

	return &custom_metrics.MetricValueList{Items: matchedMetrics}
}

// GetMetricsByName looks up metrics in the customMetricsStore by resource name.
func (s *MetricStore) GetMetricsByName(name types.NamespacedName, info provider.CustomMetricInfo) *custom_metrics.MetricValue {
	s.RLock()
	defer s.RUnlock()

	metrics, ok := s.customMetricsStore[info.Metric]
	if !ok {
		return nil
	}

	group, ok := metrics[info.GroupResource]
	if !ok {
		return nil
	}

	if !info.Namespaced {
		// TODO: rethink no namespace queries
		for _, metricMap := range group {
			if metric, ok := metricMap[name.Name]; ok {
				return &metric.Value
			}
		}
	} else if metricMap, ok := group[name.Namespace]; ok {
		if metric, ok := metricMap[name.Name]; ok {
			return &metric.Value
		}
	}

	return nil
}

// ListAllMetrics lists all custom metrics in the Metrics Store.
func (s *MetricStore) ListAllMetrics() []provider.CustomMetricInfo {
	s.RLock()
	defer s.RUnlock()

	metrics := make([]provider.CustomMetricInfo, 0, len(s.customMetricsStore))

	for metric, customMetricsStoredMetrics := range s.customMetricsStore {
		for groupResource, group := range customMetricsStoredMetrics {
			for namespace := range group {
				metric := provider.CustomMetricInfo{
					GroupResource: groupResource,
					Namespaced:    namespace != "",
					Metric:        metric,
				}
				metrics = append(metrics, metric)
			}
		}
	}

	return metrics
}

// GetExternalMetric gets external metric from the store by metric name and
// selector.
func (s *MetricStore) GetExternalMetric(namespace string, selector labels.Selector, info provider.ExternalMetricInfo) (*external_metrics.ExternalMetricValueList, error) {
	matchedMetrics := make([]external_metrics.ExternalMetricValue, 0)

	s.RLock()
	defer s.RUnlock()

	if metrics, ok := s.externalMetricsStore[info.Metric]; ok {
		for _, metric := range metrics {
			if selector.Matches(labels.Set(metric.Value.MetricLabels)) {
				matchedMetrics = append(matchedMetrics, metric.Value)
			}
		}
	}

	return &external_metrics.ExternalMetricValueList{Items: matchedMetrics}, nil
}

// ListAllExternalMetrics lists all external metrics in the Metrics Store.
func (s *MetricStore) ListAllExternalMetrics() []provider.ExternalMetricInfo {
	s.RLock()
	defer s.RUnlock()

	metricsInfo := make([]provider.ExternalMetricInfo, 0, len(s.externalMetricsStore))

	for metricName := range s.externalMetricsStore {
		info := provider.ExternalMetricInfo{
			Metric: metricName,
		}
		metricsInfo = append(metricsInfo, info)
	}
	return metricsInfo
}

// RemoveExpired removes expired metrics from the Metrics Store. A metric is
// considered expired if its metricsTTL is before time.Now().
func (s *MetricStore) RemoveExpired() {
	s.Lock()
	defer s.Unlock()

	// cleanup custom metrics
	for metricName, groups := range s.customMetricsStore {
		for group, namespaces := range groups {
			for namespace, resources := range namespaces {
				for resource, metric := range resources {
					if metric.TTL.Before(time.Now().UTC()) {
						delete(resources, resource)
					}
				}
				if len(resources) == 0 {
					delete(namespaces, namespace)
				}
			}
			if len(namespaces) == 0 {
				delete(groups, group)
			}
		}
		if len(groups) == 0 {
			delete(s.customMetricsStore, metricName)
		}
	}

	// cleanup external metrics
	for metricName, metrics := range s.externalMetricsStore {
		for k, metric := range metrics {
			if metric.TTL.Before(time.Now().UTC()) {
				delete(metrics, k)
			}
		}
		if len(metrics) == 0 {
			delete(s.externalMetricsStore, metricName)
		}
	}
}
