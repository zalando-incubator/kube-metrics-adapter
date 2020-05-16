package httpmetrics

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net"
	"net/http"
	"net/url"
	"time"

	"github.com/oliveagle/jsonpath"
)

// JSONPathMetricsGetter is a metrics getter which looks up pod metrics by
// querying the pods metrics endpoint and lookup the metric value as defined by
// the json path query.
type JSONPathMetricsGetter struct {
	jsonPath   *jsonpath.Compiled
	aggregator AggregatorFunc
	client     *http.Client
}

// NewJSONPathMetricsGetter initializes a new JSONPathMetricsGetter.
func NewJSONPathMetricsGetter(httpClient *http.Client, aggregatorFunc AggregatorFunc, compiledPath *jsonpath.Compiled) *JSONPathMetricsGetter {
	return &JSONPathMetricsGetter{client: httpClient, aggregator: aggregatorFunc, jsonPath: compiledPath}
}

var DefaultRequestTimeout = 15 * time.Second
var DefaultConnectTimeout = 15 * time.Second

func CustomMetricsHTTPClient(requestTimeout time.Duration, connectTimeout time.Duration) *http.Client {
	client := &http.Client{
		Transport: &http.Transport{
			DialContext: (&net.Dialer{
				Timeout: connectTimeout,
			}).DialContext,
			MaxIdleConns:          50,
			IdleConnTimeout:       90 * time.Second,
			ExpectContinueTimeout: 1 * time.Second,
		},
		Timeout: requestTimeout,
	}
	return client
}

func DefaultMetricsHTTPClient() *http.Client {
	return CustomMetricsHTTPClient(DefaultRequestTimeout, DefaultConnectTimeout)
}

// GetMetric gets metric from pod by fetching json metrics from the pods metric
// endpoint and extracting the desired value using the specified json path
// query.
func (g *JSONPathMetricsGetter) GetMetric(metricsURL url.URL) (float64, error) {
	data, err := g.fetchMetrics(metricsURL)
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
	case []interface{}:
		if g.aggregator == nil {
			return 0, fmt.Errorf("no aggregator function has been specified")
		}
		s, err := castSlice(res)
		if err != nil {
			return 0, err
		}
		return g.aggregator(s...), nil
	default:
		return 0, fmt.Errorf("unsupported type %T", res)
	}
}

// castSlice takes a slice of interface and returns a slice of float64 if all
// values in slice were castable, else returns an error
func castSlice(in []interface{}) ([]float64, error) {
	var out []float64

	for _, v := range in {
		switch v := v.(type) {
		case int:
			out = append(out, float64(v))
		case float32:
			out = append(out, float64(v))
		case float64:
			out = append(out, v)
		default:
			return nil, fmt.Errorf("slice was returned by JSONPath, but value inside is unsupported: %T", v)
		}
	}

	return out, nil
}

func (g *JSONPathMetricsGetter) fetchMetrics(metricsURL url.URL) ([]byte, error) {
	request, err := http.NewRequest(http.MethodGet, metricsURL.String(), nil)
	if err != nil {
		return nil, err
	}

	resp, err := g.client.Do(request)
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
