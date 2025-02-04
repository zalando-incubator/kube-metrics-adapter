package httpmetrics

import (
	"fmt"
	"net/url"
	"strconv"
	"time"

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
		jsonPath   string
		jsonEval   string
		aggregator AggregatorFunc
		err        error
	)

	if v, ok := config["json-key"]; ok {
		jsonPath = v
	}

	if v, ok := config["json-eval"]; ok {
		jsonEval = v
	}

	if jsonPath == "" && jsonEval == "" {
		return nil, fmt.Errorf("config value json-key or json-eval must be set")
	} else if jsonPath != "" && jsonEval != "" {
		return nil, fmt.Errorf("config value json-key and json-eval are mutually exclusive")
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

	requestTimeout := DefaultRequestTimeout
	connectTimeout := DefaultConnectTimeout

	if v, ok := config["request-timeout"]; ok {
		d, err := time.ParseDuration(v)
		if err != nil {
			return nil, err
		}
		if d < 0 {
			return nil, fmt.Errorf("Invalid request-timeout config value: %s", v)
		}
		requestTimeout = d
	}

	if v, ok := config["connect-timeout"]; ok {
		d, err := time.ParseDuration(v)
		if err != nil {
			return nil, err
		}
		if d < 0 {
			return nil, fmt.Errorf("Invalid connect-timeout config value: %s", v)
		}
		connectTimeout = d
	}

	jsonPathGetter, err := NewJSONPathMetricsGetter(CustomMetricsHTTPClient(requestTimeout, connectTimeout), aggregator, jsonPath, jsonEval)
	if err != nil {
		return nil, err
	}
	getter.metricGetter = jsonPathGetter
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
