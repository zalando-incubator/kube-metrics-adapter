package httpmetrics

import (
	"fmt"
	"testing"

	"github.com/oliveagle/jsonpath"
	"github.com/stretchr/testify/require"
)

func compareMetricsGetter(t *testing.T, first, second *PodMetricsJSONPathGetter) {
	require.Equal(t, first.metricGetter.jsonPath, second.metricGetter.jsonPath)
	require.Equal(t, first.scheme, second.scheme)
	require.Equal(t, first.path, second.path)
	require.Equal(t, first.port, second.port)
}

func TestNewPodJSONPathMetricsGetter(t *testing.T) {
	configNoAggregator := map[string]string{
		"json-key": "$.value",
		"scheme":   "http",
		"path":     "/metrics",
		"port":     "9090",
	}
	jpath1, _ := jsonpath.Compile(configNoAggregator["json-key"])
	getterNoAggregator, err1 := NewPodMetricsJSONPathGetter(configNoAggregator)

	require.NoError(t, err1)
	compareMetricsGetter(t, &PodMetricsJSONPathGetter{
		metricGetter: &JSONPathMetricsGetter{jsonPath: jpath1},
		scheme:       "http",
		path:         "/metrics",
		port:         9090,
	}, getterNoAggregator)

	configAggregator := map[string]string{
		"json-key":   "$.values",
		"scheme":     "http",
		"path":       "/metrics",
		"port":       "9090",
		"aggregator": "avg",
	}
	jpath2, _ := jsonpath.Compile(configAggregator["json-key"])
	getterAggregator, err2 := NewPodMetricsJSONPathGetter(configAggregator)

	require.NoError(t, err2)
	compareMetricsGetter(t, &PodMetricsJSONPathGetter{
		metricGetter: &JSONPathMetricsGetter{jsonPath: jpath2, aggregator: Average},
		scheme:       "http",
		path:         "/metrics",
		port:         9090,
	}, getterAggregator)

	configErrorJSONPath := map[string]string{
		"json-key": "{}",
		"scheme":   "http",
		"path":     "/metrics",
		"port":     "9090",
	}

	_, err3 := NewPodMetricsJSONPathGetter(configErrorJSONPath)
	require.Error(t, err3)

	configErrorPort := map[string]string{
		"json-key": "$.values",
		"scheme":   "http",
		"path":     "/metrics",
		"port":     "a9090",
	}

	_, err4 := NewPodMetricsJSONPathGetter(configErrorPort)
	require.Error(t, err4)

	configWithRawQuery := map[string]string{
		"json-key":  "$.values",
		"scheme":    "http",
		"path":      "/metrics",
		"port":      "9090",
		"raw-query": "foo=bar&baz=bop",
	}
	jpath5, _ := jsonpath.Compile(configWithRawQuery["json-key"])
	getterWithRawQuery, err5 := NewPodMetricsJSONPathGetter(configWithRawQuery)

	require.NoError(t, err5)
	compareMetricsGetter(t, &PodMetricsJSONPathGetter{
		metricGetter: &JSONPathMetricsGetter{jsonPath: jpath5},
		scheme:       "http",
		path:         "/metrics",
		port:         9090,
		rawQuery:     "foo=bar&baz=bop",
	}, getterWithRawQuery)
}

func TestBuildMetricsURL(t *testing.T) {
	scheme := "http"
	ip := "1.2.3.4"
	port := "9090"
	path := "/v1/test/"
	rawQuery := "foo=bar&baz=bop"

	// Test building URL with rawQuery
	configWithRawQuery := map[string]string{
		"json-key":  "$.value",
		"scheme":    scheme,
		"path":      path,
		"port":      port,
		"raw-query": rawQuery,
	}
	_, err := jsonpath.Compile(configWithRawQuery["json-key"])
	require.NoError(t, err)
	getterWithRawQuery, err1 := NewPodMetricsJSONPathGetter(configWithRawQuery)
	require.NoError(t, err1)

	expectedURLWithQuery := fmt.Sprintf("%s://%s:%s%s?%s", scheme, ip, port, path, rawQuery)
	receivedURLWithQuery := getterWithRawQuery.buildMetricsURL(ip)
	require.Equal(t, receivedURLWithQuery.String(), expectedURLWithQuery)

	// Test building URL without rawQuery
	configWithNoQuery := map[string]string{
		"json-key": "$.value",
		"scheme":   scheme,
		"path":     path,
		"port":     port,
	}
	_, err2 := jsonpath.Compile(configWithNoQuery["json-key"])
	require.NoError(t, err2)
	getterWithNoQuery, err3 := NewPodMetricsJSONPathGetter(configWithNoQuery)
	require.NoError(t, err3)

	expectedURLNoQuery := fmt.Sprintf("%s://%s:%s%s", scheme, ip, port, path)
	receivedURLNoQuery := getterWithNoQuery.buildMetricsURL(ip)
	require.Equal(t, receivedURLNoQuery.String(), expectedURLNoQuery)
}
