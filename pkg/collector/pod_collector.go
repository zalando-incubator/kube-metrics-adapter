package collector

import (
	"context"
	"fmt"
	"net/http"
	"time"

	log "github.com/sirupsen/logrus"
	"github.com/zalando-incubator/kube-metrics-adapter/pkg/collector/httpmetrics"
	autoscalingv2 "k8s.io/api/autoscaling/v2beta2"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/client-go/kubernetes"
	"k8s.io/metrics/pkg/apis/custom_metrics"
)

type PodCollectorPlugin struct {
	client kubernetes.Interface
}

func NewPodCollectorPlugin(client kubernetes.Interface) *PodCollectorPlugin {
	return &PodCollectorPlugin{
		client: client,
	}
}

func (p *PodCollectorPlugin) NewCollector(hpa *autoscalingv2.HorizontalPodAutoscaler, config *MetricConfig, interval time.Duration) (Collector, error) {
	return NewPodCollector(p.client, hpa, config, interval)
}

type PodCollector struct {
	client           kubernetes.Interface
	Getter           httpmetrics.PodMetricsGetter
	podLabelSelector *metav1.LabelSelector
	namespace        string
	metric           autoscalingv2.MetricIdentifier
	metricType       autoscalingv2.MetricSourceType
	interval         time.Duration
	logger           *log.Entry
	httpClient       *http.Client
}

func NewPodCollector(client kubernetes.Interface, hpa *autoscalingv2.HorizontalPodAutoscaler, config *MetricConfig, interval time.Duration) (*PodCollector, error) {
	// get pod selector based on HPA scale target ref
	selector, err := getPodLabelSelector(client, hpa)
	if err != nil {
		return nil, fmt.Errorf("failed to get pod label selector: %v", err)
	}

	c := &PodCollector{
		client:           client,
		namespace:        hpa.Namespace,
		metric:           config.Metric,
		metricType:       config.Type,
		interval:         interval,
		podLabelSelector: selector,
		logger:           log.WithFields(log.Fields{"Collector": "Pod"}),
	}

	var getter httpmetrics.PodMetricsGetter
	switch config.CollectorType {
	case "json-path":
		var err error
		getter, err = httpmetrics.NewPodMetricsJSONPathGetter(config.Config)
		if err != nil {
			return nil, err
		}
	default:
		return nil, fmt.Errorf("format '%s' not supported", config.CollectorType)
	}

	c.Getter = getter

	return c, nil
}

func (c *PodCollector) GetMetrics() ([]CollectedMetric, error) {
	opts := metav1.ListOptions{
		LabelSelector: labels.Set(c.podLabelSelector.MatchLabels).String(),
	}

	pods, err := c.client.CoreV1().Pods(c.namespace).List(context.TODO(), opts)
	if err != nil {
		return nil, err
	}

	ch := make(chan CollectedMetric)
	errCh := make(chan error)
	for _, pod := range pods.Items {
		go c.getPodMetric(pod, ch, errCh)
	}

	values := make([]CollectedMetric, 0, len(pods.Items))
	for i := 0; i < len(pods.Items); i++ {
		select {
		case err := <-errCh:
			c.logger.Error(err)
		case resp := <-ch:
			values = append(values, resp)
		}
	}

	return values, nil
}

func (c *PodCollector) Interval() time.Duration {
	return c.interval
}

func (c *PodCollector) getPodMetric(pod corev1.Pod, ch chan CollectedMetric, errCh chan error) {
	value, err := c.Getter.GetMetric(&pod)
	if err != nil {
		errCh <- fmt.Errorf("Failed to get metrics from pod '%s/%s': %v", pod.Namespace, pod.Name, err)
		return
	}

	ch <- CollectedMetric{
		Type: c.metricType,
		Custom: custom_metrics.MetricValue{
			DescribedObject: custom_metrics.ObjectReference{
				APIVersion: "v1",
				Kind:       "Pod",
				Name:       pod.Name,
				Namespace:  pod.Namespace,
			},
			Metric:    custom_metrics.MetricIdentifier{Name: c.metric.Name, Selector: c.podLabelSelector},
			Timestamp: metav1.Time{Time: time.Now().UTC()},
			Value:     *resource.NewMilliQuantity(int64(value*1000), resource.DecimalSI),
		},
	}
}

func getPodLabelSelector(client kubernetes.Interface, hpa *autoscalingv2.HorizontalPodAutoscaler) (*metav1.LabelSelector, error) {
	switch hpa.Spec.ScaleTargetRef.Kind {
	case "Deployment":
		deployment, err := client.AppsV1().Deployments(hpa.Namespace).Get(context.TODO(), hpa.Spec.ScaleTargetRef.Name, metav1.GetOptions{})
		if err != nil {
			return nil, err
		}
		return deployment.Spec.Selector, nil
	case "StatefulSet":
		sts, err := client.AppsV1().StatefulSets(hpa.Namespace).Get(context.TODO(), hpa.Spec.ScaleTargetRef.Name, metav1.GetOptions{})
		if err != nil {
			return nil, err
		}
		return sts.Spec.Selector, nil
	}

	return nil, fmt.Errorf("unable to get pod label selector for scale target ref '%s'", hpa.Spec.ScaleTargetRef.Kind)
}
