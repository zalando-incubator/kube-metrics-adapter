package provider

import (
	"fmt"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/kubernetes-sigs/custom-metrics-apiserver/pkg/provider"
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
	// metricName -> referencedResource -> objectNamespace -> objectName -> metric
	customMetricsStore customMetricStore
	// namespace -> metricName -> labels -> metric
	externalMetricsStore externalMetricStore
	metricsTTLCalculator func() time.Time
	sync.RWMutex
}

type metricName string
type objectNamespace string
type objectName string
type labelsHash string

type customMetricStore map[metricName]groupToNamespaceStore
type groupToNamespaceStore map[schema.GroupResource]namespaceToObjectStore
type namespaceToObjectStore map[objectNamespace]objectToLabelsHashStore
type objectToLabelsHashStore map[objectName]labelsHashToCustomMetricStore
type labelsHashToCustomMetricStore map[labelsHash]customMetricsStoredMetric

type externalMetricStore map[objectNamespace]namespacesTolabelsHashStore
type namespacesTolabelsHashStore map[metricName]labelsHashToExternalMetricStore
type labelsHashToExternalMetricStore map[labelsHash]externalMetricsStoredMetric

// NewMetricStore initializes an empty Metrics Store.
func NewMetricStore(ttlCalculator func() time.Time) *MetricStore {
	return &MetricStore{
		customMetricsStore:   make(customMetricStore, 0),
		externalMetricsStore: make(externalMetricStore, 0),
		metricsTTLCalculator: ttlCalculator,
	}
}

// Insert inserts a collected metric into the metric customMetricsStore.
func (s *MetricStore) Insert(value collector.CollectedMetric) {
	switch value.Type {
	case autoscalingv2.ObjectMetricSourceType, autoscalingv2.PodsMetricSourceType:
		s.insertCustomMetric(value.Custom)
	case autoscalingv2.ExternalMetricSourceType:
		s.insertExternalMetric(objectNamespace(value.Namespace), value.External)
	}
}

// insertCustomMetric inserts a custom metric plus labels into the store.
func (s *MetricStore) insertCustomMetric(value custom_metrics.MetricValue) {
	s.Lock()
	defer s.Unlock()

	// TODO: handle this mapping nicer. This information should be
	// registered as the metrics are.
	var groupResource schema.GroupResource
	switch value.DescribedObject.Kind {
	case "Pod":
		groupResource = schema.GroupResource{
			Resource: "pods",
		}
	case "Ingress":
		// group can be either `extensions` or `networking.k8s.io`
		group := "extensions"
		gv, err := schema.ParseGroupVersion(value.DescribedObject.APIVersion)
		if err == nil {
			group = gv.Group
		}
		groupResource = schema.GroupResource{
			Resource: "ingresses",
			Group:    group,
		}
	case "RouteGroup":
		group := "zalando.org"
		gv, err := schema.ParseGroupVersion(value.DescribedObject.APIVersion)
		if err == nil {
			group = gv.Group
		}
		groupResource = schema.GroupResource{
			Resource: "routegroups",
			Group:    group,
		}
	case "ScalingSchedule":
		group := "zalando.org"
		gv, err := schema.ParseGroupVersion(value.DescribedObject.APIVersion)
		if err == nil {
			group = gv.Group
		}
		groupResource = schema.GroupResource{
			Resource: "scalingschedules",
			Group:    group,
		}
	case "ClusterScalingSchedule":
		group := "zalando.org"
		gv, err := schema.ParseGroupVersion(value.DescribedObject.APIVersion)
		if err == nil {
			group = gv.Group
		}
		groupResource = schema.GroupResource{
			Resource: "clusterscalingschedules",
			Group:    group,
		}
	}

	customMetric := customMetricsStoredMetric{
		Value: value,
		TTL:   s.metricsTTLCalculator(), // TODO: make TTL configurable
	}

	selector := value.Metric.Selector
	labelsKey := labelsHash("")
	if selector != nil {
		labelsKey = hashLabelMap(selector.MatchLabels)
	}

	metric := metricName(value.Metric.Name)
	namespace := objectNamespace(value.DescribedObject.Namespace)
	object := objectName(value.DescribedObject.Name)

	group2namespace, ok := s.customMetricsStore[metric]
	if !ok {
		s.customMetricsStore[metric] = groupToNamespaceStore{
			groupResource: {
				namespace: objectToLabelsHashStore{
					object: labelsHashToCustomMetricStore{
						labelsKey: customMetric,
					},
				},
			},
		}
		return
	}

	namespace2object, ok := group2namespace[groupResource]
	if !ok {
		group2namespace[groupResource] = namespaceToObjectStore{
			namespace: {
				object: labelsHashToCustomMetricStore{
					labelsKey: customMetric,
				},
			},
		}
		return
	}

	object2label, ok := namespace2object[namespace]
	if !ok {
		namespace2object[namespace] = objectToLabelsHashStore{
			object: labelsHashToCustomMetricStore{
				labelsKey: customMetric,
			},
		}
		return
	}

	labels2metric, ok := object2label[object]
	if !ok {
		object2label[object] = labelsHashToCustomMetricStore{
			labelsKey: customMetric,
		}
		return
	}

	labels2metric[labelsKey] = customMetric
}

// insertExternalMetric inserts an external metric into the store.
func (s *MetricStore) insertExternalMetric(namespace objectNamespace, metric external_metrics.ExternalMetricValue) {
	s.Lock()
	defer s.Unlock()

	storedMetric := externalMetricsStoredMetric{
		Value: metric,
		TTL:   s.metricsTTLCalculator(), // TODO: make TTL configurable
	}

	labelsKey := hashLabelMap(metric.MetricLabels)

	metricName := metricName(metric.MetricName)

	if metrics, ok := s.externalMetricsStore[namespace]; ok {
		if labels, ok := metrics[metricName]; ok {
			labels[labelsKey] = storedMetric
		} else {
			metrics[metricName] = labelsHashToExternalMetricStore{
				labelsKey: storedMetric,
			}
		}
	} else {
		s.externalMetricsStore[namespace] = namespacesTolabelsHashStore{
			metricName: {
				labelsKey: storedMetric,
			},
		}
	}
}

// hashLabelMap converts a map into a sorted string to provide a stable
// representation of a labels map.
func hashLabelMap(labels map[string]string) labelsHash {
	strLabels := make([]string, 0, len(labels))
	for k, v := range labels {
		strLabels = append(strLabels, fmt.Sprintf("%s=%s", k, v))
	}
	sort.Strings(strLabels)
	return labelsHash(strings.Join(strLabels, ","))
}

func parseHashLabelMap(s labelsHash) labels.Set {
	labels := map[string]string{}

	if s == "" {
		return labels
	}

	keyValues := strings.Split(string(s), ",")

	for _, keyValue := range keyValues {
		splittedKeyValue := strings.Split(keyValue, "=")
		key, value := splittedKeyValue[0], splittedKeyValue[1]
		labels[key] = value
	}

	return labels
}

// GetMetricsBySelector gets metric from the customMetricsStore using a label selector to
// find metrics for matching resources.
func (s *MetricStore) GetMetricsBySelector(namespace objectNamespace, selector labels.Selector, info provider.CustomMetricInfo) *custom_metrics.MetricValueList {
	matchedMetrics := make([]custom_metrics.MetricValue, 0)

	s.RLock()
	defer s.RUnlock()

	group2namespace, ok := s.customMetricsStore[metricName(info.Metric)]
	if !ok {
		return &custom_metrics.MetricValueList{}
	}

	namespace2object, ok := group2namespace[info.GroupResource]
	if !ok {
		return &custom_metrics.MetricValueList{}
	}

	if !info.Namespaced {
		for _, object2labels := range namespace2object {
			for _, labels2metric := range object2labels {
				for _, metric := range labels2metric {
					if selector.Matches(labels.Set(metric.Value.Metric.Selector.MatchLabels)) {
						matchedMetrics = append(matchedMetrics, metric.Value)
					}
				}
			}
		}
	} else if object2labels, ok := namespace2object[namespace]; ok {
		for _, labels2hash := range object2labels {
			for _, metric := range labels2hash {
				if metric.Value.Metric.Selector != nil && selector.Matches(labels.Set(metric.Value.Metric.Selector.MatchLabels)) {
					matchedMetrics = append(matchedMetrics, metric.Value)
				}
			}
		}
	}

	return &custom_metrics.MetricValueList{Items: matchedMetrics}
}

// GetMetricsByName looks up metrics in the customMetricsStore by resource name.
func (s *MetricStore) GetMetricsByName(object types.NamespacedName, info provider.CustomMetricInfo, selector labels.Selector) *custom_metrics.MetricValue {
	name := objectName(object.Name)
	namespace := objectNamespace(object.Namespace)

	s.RLock()
	defer s.RUnlock()

	group2namespace, ok := s.customMetricsStore[metricName(info.Metric)]
	if !ok {
		return nil
	}

	namespace2object, ok := group2namespace[info.GroupResource]
	if !ok {
		return nil
	}

	if !info.Namespaced {
		// TODO: rethink no namespace queries
		namespace := objectNamespace(name)

		for _, object2label := range namespace2object {
			if label2metric, ok := object2label[objectName(namespace)]; ok {
				for metric, value := range label2metric {
					if selector.Matches(parseHashLabelMap(metric)) {
						return &value.Value
					}
				}
			}
		}
	} else if object2label, ok := namespace2object[namespace]; ok {
		if label2metric, ok := object2label[name]; ok {
			for metric, value := range label2metric {
				if selector.Matches(parseHashLabelMap(metric)) {
					return &value.Value
				}
			}
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
					Metric:        string(metric),
				}
				metrics = append(metrics, metric)
			}
		}
	}

	return metrics
}

// GetExternalMetric gets external metric from the store by metric name and
// selector.
func (s *MetricStore) GetExternalMetric(namespace objectNamespace, selector labels.Selector, info provider.ExternalMetricInfo) (*external_metrics.ExternalMetricValueList, error) {
	matchedMetrics := make([]external_metrics.ExternalMetricValue, 0)

	s.RLock()
	defer s.RUnlock()

	if metrics, ok := s.externalMetricsStore[namespace]; ok {
		if selectors, ok := metrics[metricName(info.Metric)]; ok {
			for _, sel := range selectors {
				if selector.Matches(labels.Set(sel.Value.MetricLabels)) {
					matchedMetrics = append(matchedMetrics, sel.Value)
				}
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

	for _, metrics := range s.externalMetricsStore {
		for metricName := range metrics {
			info := provider.ExternalMetricInfo{
				Metric: string(metricName),
			}
			metricsInfo = append(metricsInfo, info)
		}
	}
	return metricsInfo
}

// RemoveExpired removes expired metrics from the Metrics Store. A metric is
// considered expired if its metricsTTL is before time.Now().
func (s *MetricStore) RemoveExpired() {
	s.Lock()
	defer s.Unlock()

	// cleanup custom metrics
	for metricName, group2namespace := range s.customMetricsStore {
		for group, namespace2object := range group2namespace {
			for namespace, object2label := range namespace2object {
				for object, label2metric := range object2label {
					for labelsHash, metric := range label2metric {
						if metric.TTL.Before(time.Now().UTC()) {
							delete(label2metric, labelsHash)
						}
					}
					if len(label2metric) == 0 {
						delete(object2label, object)
					}
				}
				if len(object2label) == 0 {
					delete(namespace2object, namespace)
				}
			}
			if len(namespace2object) == 0 {
				delete(group2namespace, group)
			}
		}
		if len(group2namespace) == 0 {
			delete(s.customMetricsStore, metricName)
		}
	}

	// cleanup external metrics
	for namespace, metrics := range s.externalMetricsStore {
		for metricName, selectors := range metrics {
			for k, metric := range selectors {
				if metric.TTL.Before(time.Now().UTC()) {
					delete(selectors, k)
				}
			}
			if len(selectors) == 0 {
				delete(metrics, metricName)
			}
		}
		if len(metrics) == 0 {
			delete(s.externalMetricsStore, namespace)
		}
	}
}
