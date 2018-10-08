package collector

import autoscalingv2beta1 "k8s.io/api/autoscaling/v2beta1"

type ObjectMetricsGetter interface {
	GetObjectMetric(namespace string, reference *autoscalingv2beta1.CrossVersionObjectReference) (float64, error)
}

// type PodCollector struct {
// 	client           kubernetes.Interface
// 	Getter           PodMetricsGetter
// 	podLabelSelector string
// 	namespace        string
// 	metricName       string
// 	interval         time.Duration
// }

// func NewObjectCollector(client kubernetes.Interface, hpa *autoscalingv2beta1.HorizontalPodAutoscaler, metricName string, config *MetricConfig, interval time.Duration) (Collector, error) {
// 	switch
// }
