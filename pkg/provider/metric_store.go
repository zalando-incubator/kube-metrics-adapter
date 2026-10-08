package provider

import (
	"context"
	"encoding/json"
	"sync"
	"time"

	"github.com/zalando-incubator/kube-metrics-adapter/pkg/collector"
	autoscalingv2 "k8s.io/api/autoscaling/v2"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/metrics/pkg/apis/custom_metrics"
	"k8s.io/metrics/pkg/apis/external_metrics"
	"sigs.k8s.io/custom-metrics-apiserver/pkg/provider"
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
type labelsKey string

type customMetricStore map[metricName]groupToNamespaceStore
type groupToNamespaceStore map[schema.GroupResource]namespaceToObjectStore
type namespaceToObjectStore map[objectNamespace]objectToLabelsKeyStore
type objectToLabelsKeyStore map[objectName]labelsKeyToCustomMetricStore
type labelsKeyToCustomMetricStore map[labelsKey]customMetricsStoredMetric

type externalMetricStore map[objectNamespace]namespacesToLabelsKeyStore
type namespacesToLabelsKeyStore map[metricName]labelsKeyToExternalMetricStore
type labelsKeyToExternalMetricStore map[labelsKey]externalMetricsStoredMetric

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
		group := "networking.k8s.io"
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

	labelsKey := labelSelectorKey(value.Metric.Selector)

	metric := metricName(value.Metric.Name)
	namespace := objectNamespace(value.DescribedObject.Namespace)
	object := objectName(value.DescribedObject.Name)

	group2namespace, ok := s.customMetricsStore[metric]
	if !ok {
		s.customMetricsStore[metric] = groupToNamespaceStore{
			groupResource: {
				namespace: objectToLabelsKeyStore{
					object: labelsKeyToCustomMetricStore{
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
				object: labelsKeyToCustomMetricStore{
					labelsKey: customMetric,
				},
			},
		}
		return
	}

	object2label, ok := namespace2object[namespace]
	if !ok {
		namespace2object[namespace] = objectToLabelsKeyStore{
			object: labelsKeyToCustomMetricStore{
				labelsKey: customMetric,
			},
		}
		return
	}

	labels2metric, ok := object2label[object]
	if !ok {
		object2label[object] = labelsKeyToCustomMetricStore{
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

	labelsKey := labelSetKey(metric.MetricLabels)

	metricName := metricName(metric.MetricName)

	if metrics, ok := s.externalMetricsStore[namespace]; ok {
		if labels, ok := metrics[metricName]; ok {
			labels[labelsKey] = storedMetric
		} else {
			metrics[metricName] = labelsKeyToExternalMetricStore{
				labelsKey: storedMetric,
			}
		}
	} else {
		s.externalMetricsStore[namespace] = namespacesToLabelsKeyStore{
			metricName: {
				labelsKey: storedMetric,
			},
		}
	}
}

// labelSetKey returns the canonical string representation of a label set.
func labelSetKey(metricLabels map[string]string) labelsKey {
	return labelsKey(labels.Set(metricLabels).String())
}

// labelSelectorKey creates a stable key for the full metric selector.
// MatchExpressions are part of the selector identity too; keying only on
// MatchLabels causes distinct expression selectors to overwrite each other.
func labelSelectorKey(selector *metav1.LabelSelector) labelsKey {
	if selector == nil {
		return labelsKey("")
	}

	parsed, err := metav1.LabelSelectorAsSelector(selector)
	if err == nil {
		return labelsKey(parsed.String())
	}

	// Invalid selectors should be rejected by Kubernetes validation, but keep
	// distinct invalid values separate if one reaches the in-memory store.
	encoded, _ := json.Marshal(selector)
	return labelsKey(string(encoded))
}

// metricSelectorMatches matches a request selector against a stored metric.
// MatchLabels represent concrete metric labels and support normal label
// selector matching. When the stored selector contains MatchExpressions, it
// is the metric identity echoed by the collector rather than a concrete label
// set; in that case only an equivalent request selector (or an empty selector)
// identifies the stored metric.
func metricSelectorMatches(requestSelector labels.Selector, metricSelector *metav1.LabelSelector) bool {
	if requestSelector == nil || requestSelector.Empty() {
		return true
	}
	if metricSelector == nil {
		return false
	}

	if len(metricSelector.MatchExpressions) > 0 {
		parsed, err := metav1.LabelSelectorAsSelector(metricSelector)
		return err == nil && requestSelector.String() == parsed.String()
	}

	return requestSelector.Matches(labels.Set(metricSelector.MatchLabels))
}

// GetMetricsBySelector gets metric from the customMetricsStore using a label selector to
// find metrics for matching resources.
func (s *MetricStore) GetMetricsBySelector(_ context.Context, namespace objectNamespace, selector labels.Selector, info provider.CustomMetricInfo) *custom_metrics.MetricValueList {
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
					if metricSelectorMatches(selector, metric.Value.Metric.Selector) {
						matchedMetrics = append(matchedMetrics, metric.Value)
					}
				}
			}
		}
	} else if object2labels, ok := namespace2object[namespace]; ok {
		for _, labels2hash := range object2labels {
			for _, metric := range labels2hash {
				if metricSelectorMatches(selector, metric.Value.Metric.Selector) {
					matchedMetrics = append(matchedMetrics, metric.Value)
				}
			}
		}
	}

	return &custom_metrics.MetricValueList{Items: matchedMetrics}
}

// GetMetricsByName looks up metrics in the customMetricsStore by resource name.
func (s *MetricStore) GetMetricsByName(_ context.Context, object types.NamespacedName, info provider.CustomMetricInfo, selector labels.Selector) *custom_metrics.MetricValue {
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
				for _, value := range label2metric {
					if metricSelectorMatches(selector, value.Value.Metric.Selector) {
						return &value.Value
					}
				}
			}
		}
	} else if object2label, ok := namespace2object[namespace]; ok {
		if label2metric, ok := object2label[name]; ok {
			for _, value := range label2metric {
				if metricSelectorMatches(selector, value.Value.Metric.Selector) {
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
func (s *MetricStore) GetExternalMetric(_ context.Context, namespace objectNamespace, selector labels.Selector, info provider.ExternalMetricInfo) (*external_metrics.ExternalMetricValueList, error) {
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
					for labelsKey, metric := range label2metric {
						if metric.TTL.Before(time.Now().UTC()) {
							delete(label2metric, labelsKey)
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
