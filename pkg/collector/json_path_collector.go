package collector

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"math"
	"net/http"
	"net/url"
	"strconv"
	"time"

	"github.com/oliveagle/jsonpath"
	corev1 "k8s.io/api/core/v1"
)

// JSONPathMetricsGetter is a metrics getter which looks up pod metrics by
// querying the pods metrics endpoint and lookup the metric value as defined by
// the json path query.
type JSONPathMetricsGetter struct {
	jsonPath   *jsonpath.Compiled
	scheme     string
	path       string
	port       int
	aggregator string
}

// NewJSONPathMetricsGetter initializes a new JSONPathMetricsGetter.
func NewJSONPathMetricsGetter(config map[string]string) (*JSONPathMetricsGetter, error) {
	getter := &JSONPathMetricsGetter{}

	if v, ok := config["json-key"]; ok {
		pat, err := jsonpath.Compile(v)
		if err != nil {
			return nil, fmt.Errorf("failed to parse json path definition: %v", err)
		}

		getter.jsonPath = pat
	}

	if v, ok := config["scheme"]; ok {
		getter.scheme = v
	}

	if v, ok := config["path"]; ok {
		getter.path = v
	}

	if v, ok := config["port"]; ok {
		n, err := strconv.Atoi(v)
		if err != nil {
			return nil, err
		}
		getter.port = n
	}

	if v, ok := config["aggregator"]; ok {
		getter.aggregator = v
	}

	return getter, nil
}

// GetMetric gets metric from pod by fetching json metrics from the pods metric
// endpoint and extracting the desired value using the specified json path
// query.
func (g *JSONPathMetricsGetter) GetMetric(pod *corev1.Pod) (float64, error) {
	data, err := getPodMetrics(pod, g.scheme, g.path, g.port)
	if err != nil {
		return 0, err
	}

	// parse data
	var jsonData interface{}
	err = json.Unmarshal(data, &jsonData)
	if err != nil {
		return 0, err
	}

	res, err := g.jsonPath.Lookup(jsonData)
	if err != nil {
		return 0, err
	}

	switch res := res.(type) {
	case int:
		return float64(res), nil
	case float32:
		return float64(res), nil
	case float64:
		return res, nil
	case []int:
		return reduce(intsToFloat64s(res), g.aggregator)
	case []float32:
		return reduce(float32sToFloat64s(res), g.aggregator)
	case []float64:
		return reduce(res, g.aggregator)
	default:
		return 0, fmt.Errorf("unsupported type %T", res)
	}
}

// getPodMetrics returns the content of the pods metrics endpoint.
func getPodMetrics(pod *corev1.Pod, scheme, path string, port int) ([]byte, error) {
	if pod.Status.PodIP == "" {
		return nil, fmt.Errorf("pod %s/%s does not have a pod IP", pod.Namespace, pod.Namespace)
	}

	httpClient := &http.Client{
		Timeout:   15 * time.Second,
		Transport: &http.Transport{},
	}

	if scheme == "" {
		scheme = "http"
	}

	metricsURL := url.URL{
		Scheme: scheme,
		Host:   fmt.Sprintf("%s:%d", pod.Status.PodIP, port),
		Path:   path,
	}

	request, err := http.NewRequest(http.MethodGet, metricsURL.String(), nil)
	if err != nil {
		return nil, err
	}

	resp, err := httpClient.Do(request)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	data, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("unsuccessful response: %s", resp.Status)
	}

	return data, nil
}

// intsToFloat64s will convert a slice of int to a slice of float64
func intsToFloat64s(in []int) (out []float64) {
	for i, v := range in {
		out[i] = float64(v)
	}
	return
}

// float32sToFloat64s will convert a slice of float32 to a slice of float64
func float32sToFloat64s(in []float32) (out []float64) {
	for i, v := range in {
		out[i] = float64(v)
	}
	return
}

// reduce will reduce a slice of numbers given a reducer function's name. If it's empty or not recognized, avg is used.
func reduce(values []float64, aggregator string) (float64, error) {
	switch aggregator {
	case "avg":
		return avg(values), nil
	case "min":
		return min(values), nil
	case "max":
		return max(values), nil
	case "sum":
		return sum(values), nil
	default:
		return 0, fmt.Errorf("slice of numbers was returned by JSONPath, but no valid aggregator function was specified: %v", aggregator)
	}
}

// avg implements the average mathematical function over a slice of float64
func avg(values []float64) float64 {
	sum := sum(values)
	return sum / float64(len(values))
}

// min implements the absolute minimum mathematical function over a slice of float64
func min(values []float64) float64 {
	// initialized with positive infinity, all finite numbers are smaller than it
	curMin := math.Inf(1)

	for _, v := range values {
		if v < curMin {
			curMin = v
		}
	}

	return curMin
}

// max implements the absolute maximum mathematical function over a slice of float64
func max(values []float64) float64 {
	// initialized with negative infinity, all finite numbers are bigger than it
	curMax := math.Inf(-1)

	for _, v := range values {
		if v > curMax {
			curMax = v
		}
	}

	return curMax
}

// sum implements the summation mathematical function over a slice of float64
func sum(values []float64) float64 {
	res := 0.0

	for _, v := range values {
		res += v
	}

	return res
}
