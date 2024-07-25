package collector

import (
	"context"
	"fmt"
	"time"

	argoRolloutsClient "github.com/argoproj/argo-rollouts/pkg/client/clientset/versioned"
	log "github.com/sirupsen/logrus"
	autoscalingv2 "k8s.io/api/autoscaling/v2"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/client-go/kubernetes"
	"k8s.io/metrics/pkg/apis/custom_metrics"

	"github.com/zalando-incubator/kube-metrics-adapter/pkg/collector/httpmetrics"
)

type PodCollectorPlugin struct {
	client             kubernetes.Interface
	argoRolloutsClient argoRolloutsClient.Interface
}

func NewPodCollectorPlugin(client kubernetes.Interface, argoRolloutsClient argoRolloutsClient.Interface) *PodCollectorPlugin {
	return &PodCollectorPlugin{
		client:             client,
		argoRolloutsClient: argoRolloutsClient,
	}
}

func (p *PodCollectorPlugin) NewCollector(ctx context.Context, hpa *autoscalingv2.HorizontalPodAutoscaler, config *MetricConfig, interval time.Duration) (Collector, error) {
	return NewPodCollector(ctx, p.client, p.argoRolloutsClient, hpa, config, interval)
}

type PodCollector struct {
	client           kubernetes.Interface
	Getter           httpmetrics.PodMetricsGetter
	podLabelSelector *metav1.LabelSelector
	namespace        string
	metric           autoscalingv2.MetricIdentifier
	metricType       autoscalingv2.MetricSourceType
	minPodReadyAge   time.Duration
	maxPodSampleSize int
	interval         time.Duration
	logger           *log.Entry
}

func NewPodCollector(ctx context.Context, client kubernetes.Interface, argoRolloutsClient argoRolloutsClient.Interface, hpa *autoscalingv2.HorizontalPodAutoscaler, config *MetricConfig, interval time.Duration) (*PodCollector, error) {
	// get pod selector based on HPA scale target ref
	selector, err := getPodLabelSelector(ctx, client, argoRolloutsClient, hpa)
	if err != nil {
		return nil, fmt.Errorf("failed to get pod label selector: %v", err)
	}

	c := &PodCollector{
		client:           client,
		namespace:        hpa.Namespace,
		metric:           config.Metric,
		metricType:       config.Type,
		minPodReadyAge:   config.MinPodReadyAge,
		maxPodSampleSize: config.MaxPodSampleSize,
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

func (c *PodCollector) GetMetrics(ctx context.Context) ([]CollectedMetric, error) {
	opts := metav1.ListOptions{
		LabelSelector: labels.Set(c.podLabelSelector.MatchLabels).String(),
	}

	pods, err := c.client.CoreV1().Pods(c.namespace).List(ctx, opts)
	if err != nil {
		return nil, err
	}

	ch := make(chan CollectedMetric)
	errCh := make(chan error)
	skippedPodsCount := 0
	sampledPodsCount := 0

	for _, pod := range pods.Items {

		isPodReady, podReadyAge := GetPodReadyAge(pod)

		if isPodReady {
			if pod.DeletionTimestamp != nil {
				skippedPodsCount++
				c.logger.Debugf("Skipping metrics collection for pod %s/%s because it is being terminated (DeletionTimestamp: %s)", pod.Namespace, pod.Name, pod.DeletionTimestamp)
			} else if podReadyAge < c.minPodReadyAge {
				skippedPodsCount++
				c.logger.Warnf("Skipping metrics collection for pod %s/%s because it's ready age is %s and min-pod-ready-age is set to %s", pod.Namespace, pod.Name, podReadyAge, c.minPodReadyAge)
			} else {
				go c.getPodMetric(pod, ch, errCh)
				if sampledPodsCount++; sampledPodsCount > c.maxPodSampleSize {
					break
				}
			}
		} else {
			skippedPodsCount++
			c.logger.Debugf("Skipping metrics collection for pod %s/%s because it's status is not Ready.", pod.Namespace, pod.Name)
		}
	}

	values := make([]CollectedMetric, 0, (len(pods.Items) - skippedPodsCount))
	for i := 0; i < (len(pods.Items) - skippedPodsCount); i++ {
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
		Namespace: c.namespace,
		Type:      c.metricType,
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

func getPodLabelSelector(ctx context.Context, client kubernetes.Interface, argoRolloutsClient argoRolloutsClient.Interface, hpa *autoscalingv2.HorizontalPodAutoscaler) (*metav1.LabelSelector, error) {
	switch hpa.Spec.ScaleTargetRef.Kind {
	case "Deployment":
		deployment, err := client.AppsV1().Deployments(hpa.Namespace).Get(ctx, hpa.Spec.ScaleTargetRef.Name, metav1.GetOptions{})
		if err != nil {
			return nil, err
		}
		return deployment.Spec.Selector, nil
	case "StatefulSet":
		sts, err := client.AppsV1().StatefulSets(hpa.Namespace).Get(ctx, hpa.Spec.ScaleTargetRef.Name, metav1.GetOptions{})
		if err != nil {
			return nil, err
		}
		return sts.Spec.Selector, nil
	case "Rollout":
		rollout, err := argoRolloutsClient.ArgoprojV1alpha1().Rollouts(hpa.Namespace).Get(ctx, hpa.Spec.ScaleTargetRef.Name, metav1.GetOptions{})
		if err != nil {
			return nil, err
		}
		return rollout.Spec.Selector, nil
	}

	return nil, fmt.Errorf("unable to get pod label selector for scale target ref '%s'", hpa.Spec.ScaleTargetRef.Kind)
}

// GetPodReadyAge extracts corev1.PodReady condition from the given pod object and
// returns true, time.Duration() for LastTransitionTime if the condition corev1.PodReady is found. Returns time.Duration(0s), false if the condition is not present.
func GetPodReadyAge(pod corev1.Pod) (bool, time.Duration) {
	podReadyAge := time.Duration(0 * time.Second)
	conditions := pod.Status.Conditions
	if conditions == nil {
		return false, podReadyAge
	}
	for i := range conditions {
		if conditions[i].Type == corev1.PodReady && conditions[i].Status == corev1.ConditionTrue {
			podReadyAge = time.Since(conditions[i].LastTransitionTime.Time)
			return true, podReadyAge
		}
	}

	return false, podReadyAge
}
