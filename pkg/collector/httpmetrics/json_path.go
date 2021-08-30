package httpmetrics

import (
	"fmt"
	"io/ioutil"
	"net"
	"net/http"
	"net/url"
	"time"

	"github.com/spyzhov/ajson"
)

// JSONPathMetricsGetter is a metrics getter which looks up pod metrics by
// querying the pods metrics endpoint and lookup the metric value as defined by
// the json path query.
type JSONPathMetricsGetter struct {
	jsonPath   string
	aggregator AggregatorFunc
	client     *http.Client
}

// NewJSONPathMetricsGetter initializes a new JSONPathMetricsGetter.
func NewJSONPathMetricsGetter(httpClient *http.Client, aggregatorFunc AggregatorFunc, jsonPath string) (*JSONPathMetricsGetter, error) {
	// check that jsonPath parses
	_, err := ajson.ParseJSONPath(jsonPath)
	if err != nil {
		return nil, err
	}
	return &JSONPathMetricsGetter{client: httpClient, aggregator: aggregatorFunc, jsonPath: jsonPath}, nil
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
	root, err := ajson.Unmarshal(data)
	if err != nil {
		return 0, err
	}

	nodes, err := root.JSONPath(g.jsonPath)
	if err != nil {
		return 0, err
	}

	if len(nodes) == 0 {
		return 0, fmt.Errorf("unexpected json: expected single numeric or array value")
	}

	if len(nodes) != 1 {
		nodes = []*ajson.Node{ajson.ArrayNode("root", nodes)}
	}

	node := nodes[0]
	if node.IsArray() {
		if g.aggregator == nil {
			return 0, fmt.Errorf("no aggregator function has been specified")
		}
		values := make([]float64, 0, len(nodes))
		items, _ := node.GetArray()
		for _, item := range items {
			value, err := item.GetNumeric()
			if err != nil {
				return 0, fmt.Errorf("did not find numeric type: %w", err)
			}
			values = append(values, value)
		}
		return g.aggregator(values...), nil
	} else if node.IsNumeric() {
		res, _ := node.GetNumeric()
		return res, nil
	}

	value, err := node.Value()
	if err != nil {
		return 0, fmt.Errorf("failed to check value of jsonPath result: %w", err)
	}
	return 0, fmt.Errorf("unsupported type %T", value)
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
