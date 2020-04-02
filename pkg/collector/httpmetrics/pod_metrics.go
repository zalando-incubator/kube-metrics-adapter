package httpmetrics

import (
	"fmt"
	"net/url"
	"strconv"

	"github.com/oliveagle/jsonpath"
	v1 "k8s.io/api/core/v1"
)

type PodMetricsGetter interface {
	GetMetric(pod *v1.Pod) (float64, error)
}

type PodMetricsJSONPathGetter struct {
	scheme       string
	path         string
	rawQuery     string
	port         int
	metricGetter *JSONPathMetricsGetter
}

func (g PodMetricsJSONPathGetter) GetMetric(pod *v1.Pod) (float64, error) {
	if pod.Status.PodIP == "" {
		return 0, fmt.Errorf("pod %s/%s does not have a pod IP", pod.Namespace, pod.Name)
	}
	metricsURL := g.buildMetricsURL(pod.Status.PodIP)
	return g.metricGetter.GetMetric(metricsURL)
}

func NewPodMetricsJSONPathGetter(config map[string]string) (*PodMetricsJSONPathGetter, error) {
	getter := PodMetricsJSONPathGetter{}
	var (
		jsonPath   *jsonpath.Compiled
		aggregator AggregatorFunc
		err        error
	)

	if v, ok := config["json-key"]; ok {
		path, err := jsonpath.Compile(v)
		if err != nil {
			return nil, fmt.Errorf("failed to parse json path definition: %v", err)
		}

		jsonPath = path
	}

	if v, ok := config["scheme"]; ok {
		getter.scheme = v
	}

	if v, ok := config["path"]; ok {
		getter.path = v
	}

	if v, ok := config["raw-query"]; ok {
		getter.rawQuery = v
	}

	if v, ok := config["port"]; ok {
		n, err := strconv.Atoi(v)
		if err != nil {
			return nil, err
		}
		getter.port = n
	}

	if v, ok := config["aggregator"]; ok {
		aggregator, err = ParseAggregator(v)
		if err != nil {
			return nil, err
		}
	}
	getter.metricGetter = NewJSONPathMetricsGetter(DefaultMetricsHTTPClient(), aggregator, jsonPath)
	return &getter, nil
}

// buildMetricsURL will build the full URL needed to hit the pod metric endpoint.
func (g *PodMetricsJSONPathGetter) buildMetricsURL(podIP string) url.URL {
	var scheme = g.scheme

	if scheme == "" {
		scheme = "http"
	}

	return url.URL{
		Scheme:   scheme,
		Host:     fmt.Sprintf("%s:%d", podIP, g.port),
		Path:     g.path,
		RawQuery: g.rawQuery,
	}
}
