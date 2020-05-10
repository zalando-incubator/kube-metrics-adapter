package collector

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/oliveagle/jsonpath"
	"github.com/stretchr/testify/require"
	"k8s.io/api/autoscaling/v2beta2"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

type testExternalMetricsHandler struct {
	values []int64
	test   *testing.T
}

func (t testExternalMetricsHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	response, err := json.Marshal(testMetricResponse{t.values})
	require.NoError(t.test, err)
	w.Header().Set("Content-Type", "application/json")
	_, err = w.Write(response)
	require.NoError(t.test, err)
}

func makeHTTPTestServer(t *testing.T, values []int64) string {
	server := httptest.NewServer(&testExternalMetricsHandler{values: values, test: t})
	return server.URL
}

func TestHTTPCollector(t *testing.T) {
	for _, tc := range []struct {
		name       string
		values     []int64
		output     int
		aggregator string
	}{
		{
			name:       "basic",
			values:     []int64{3},
			output:     3,
			aggregator: "sum",
		},
		{
			name:       "sum",
			values:     []int64{3, 5, 6},
			aggregator: "sum",
			output:     14,
		},
		{
			name:       "average",
			values:     []int64{3, 5, 6},
			aggregator: "sum",
			output:     14,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			testServer := makeHTTPTestServer(t, tc.values)
			plugin, err := NewHTTPCollectorPlugin()
			require.NoError(t, err)
			testConfig := makeTestHTTPCollectorConfig(testServer, tc.aggregator)
			collector, err := plugin.NewCollector(nil, testConfig, testInterval)
			require.NoError(t, err)
			metrics, err := collector.GetMetrics()
			require.NoError(t, err)
			require.NotNil(t, metrics)
			require.Len(t, metrics, 1)
			require.EqualValues(t, metrics[0].External.Value.Value(), tc.output)
		})
	}
}

func makeTestHTTPCollectorConfig(endpoint, aggregator string) *MetricConfig {
	config := &MetricConfig{
		MetricTypeName: MetricTypeName{
			Type: v2beta2.ExternalMetricSourceType,
			Metric: v2beta2.MetricIdentifier{
				Name: "test-metric",
				Selector: &v1.LabelSelector{
					MatchLabels: map[string]string{identifierLabel: "test-metric"},
				},
			},
		},
		Config: map[string]string{
			HTTPJsonPathAnnotationKey: "$.values",
			HTTPEndpointAnnotationKey: endpoint,
		},
	}
	if aggregator != "" {
		config.Config["aggregator"] = aggregator
	}
	return config
}

func testMetricConfig(annotationPath, annotationEnpoint, annotationAgg, labelPath, labelEndpoint, labelAgg, labelIdentifier string) *MetricConfig {
	config := &MetricConfig{
		Config: map[string]string{},
		MetricTypeName: MetricTypeName{
			Metric: v2beta2.MetricIdentifier{
				Name: "http",
				Selector: &v1.LabelSelector{
					MatchLabels: map[string]string{
						identifierLabel: labelIdentifier,
					},
				},
			},
		},
	}
	if annotationPath != "" {
		config.Config[HTTPJsonPathAnnotationKey] = annotationPath
	}
	if annotationEnpoint != "" {
		config.Config[HTTPEndpointAnnotationKey] = annotationEnpoint
	}
	if annotationAgg != "" {
		config.Config[aggregatorKey] = annotationAgg
	}
	if labelPath != "" {
		config.Metric.Selector.MatchLabels[HTTPJsonPathAnnotationKey] = labelPath
	}
	if labelEndpoint != "" {
		config.Metric.Selector.MatchLabels[HTTPEndpointAnnotationKey] = labelEndpoint
	}
	if labelAgg != "" {
		config.Metric.Selector.MatchLabels[aggregatorKey] = labelAgg
	}
	return config
}

func TestNewHTTPCollectorPlugin(t *testing.T) {
	for _, tc := range []struct {
		name               string
		interval           time.Duration
		annotationPath     string
		annotationEndpoint string
		annotationAgg      string
		labelPath          string
		labelEndpoint      string
		labelAgg           string
		labelIdentifier    string
		configPath         string
		configEndpoint     string
		configAgg          string
	}{
		{
			name:               "basic",
			annotationPath:     "$.annotation",
			annotationEndpoint: "http://annotation:8081",
			annotationAgg:      "max",
			labelIdentifier:    "test-metric",
			configPath:         "$.annotation",
			configEndpoint:     "http://annotation:8081",
			configAgg:          "max",
			interval:           15 * time.Second,
		},
		{
			name:               "override",
			annotationPath:     "$.annotation",
			annotationEndpoint: "http://annotation:8081",
			annotationAgg:      "max",
			labelPath:          "$.label",
			labelEndpoint:      "http://label:8081",
			labelAgg:           "min",
			labelIdentifier:    "test-metric",
			configPath:         "$.label",
			configEndpoint:     "http://label:8081",
			configAgg:          "min",
			interval:           15 * time.Second,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			plugin, err := NewHTTPCollectorPlugin()
			require.NoError(t, err)
			config := testMetricConfig(tc.annotationPath, tc.annotationEndpoint, tc.annotationAgg, tc.labelPath, tc.labelEndpoint, tc.labelAgg, tc.labelIdentifier)
			collector, err := plugin.NewCollector(nil, config, tc.interval)
			require.NoError(t, err)
			c, ok := collector.(*HTTPCollector)
			require.True(t, ok)

			require.Equal(t, tc.interval, c.interval)
			p, err := jsonpath.Compile(tc.configPath)
			require.NoError(t, err)
			require.Equal(t, p, c.jsonPath)
			require.Equal(t, tc.configEndpoint, c.endpoint.String())
		})
	}
}
