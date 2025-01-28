package httpmetrics

import (
	"fmt"
	"testing"
	"time"

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
	getterNoAggregator, err1 := NewPodMetricsJSONPathGetter(configNoAggregator)

	require.NoError(t, err1)
	compareMetricsGetter(t, &PodMetricsJSONPathGetter{
		metricGetter: &JSONPathMetricsGetter{jsonPath: configNoAggregator["json-key"]},
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
	getterAggregator, err2 := NewPodMetricsJSONPathGetter(configAggregator)

	require.NoError(t, err2)
	compareMetricsGetter(t, &PodMetricsJSONPathGetter{
		metricGetter: &JSONPathMetricsGetter{jsonPath: configAggregator["json-key"], aggregator: Average},
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
	getterWithRawQuery, err5 := NewPodMetricsJSONPathGetter(configWithRawQuery)

	require.NoError(t, err5)
	compareMetricsGetter(t, &PodMetricsJSONPathGetter{
		metricGetter: &JSONPathMetricsGetter{jsonPath: configWithRawQuery["json-key"]},
		scheme:       "http",
		path:         "/metrics",
		port:         9090,
		rawQuery:     "foo=bar&baz=bop",
	}, getterWithRawQuery)

	configErrorMixedPathEval := map[string]string{
		"json-key": "{}",
		"json-eval": "avg($.values)",
		"scheme":   "http",
		"path":     "/metrics",
		"port":     "9090",
	}

	_, err6 := NewPodMetricsJSONPathGetter(configErrorMixedPathEval)
	require.Error(t, err6)
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
	getterWithNoQuery, err3 := NewPodMetricsJSONPathGetter(configWithNoQuery)
	require.NoError(t, err3)

	expectedURLNoQuery := fmt.Sprintf("%s://%s:%s%s", scheme, ip, port, path)
	receivedURLNoQuery := getterWithNoQuery.buildMetricsURL(ip)
	require.Equal(t, receivedURLNoQuery.String(), expectedURLNoQuery)
}

func TestCustomTimeouts(t *testing.T) {
	scheme := "http"
	port := "9090"
	path := "/v1/test/"

	// Test no custom options results in default timeouts
	defaultConfig := map[string]string{
		"json-key": "$.value",
		"scheme":   scheme,
		"path":     path,
		"port":     port,
	}
	defaultTime := time.Duration(15000) * time.Millisecond

	defaultGetter, err1 := NewPodMetricsJSONPathGetter(defaultConfig)
	require.NoError(t, err1)
	require.Equal(t, defaultGetter.metricGetter.client.Timeout, defaultTime)

	// Test with custom request timeout
	configWithRequestTimeout := map[string]string{
		"json-key":        "$.value",
		"scheme":          scheme,
		"path":            path,
		"port":            port,
		"request-timeout": "978ms",
	}
	exectedTimeout := time.Duration(978) * time.Millisecond
	customRequestGetter, err2 := NewPodMetricsJSONPathGetter(configWithRequestTimeout)
	require.NoError(t, err2)
	require.Equal(t, customRequestGetter.metricGetter.client.Timeout, exectedTimeout)

	// Test with custom connect timeout. Unfortunately, it seems there's no way to access the
	// connect timeout of the client struct to actually verify it's set :/
	configWithConnectTimeout := map[string]string{
		"json-key":        "$.value",
		"scheme":          scheme,
		"path":            path,
		"port":            port,
		"connect-timeout": "512ms",
	}
	_, err3 := NewPodMetricsJSONPathGetter(configWithConnectTimeout)
	require.NoError(t, err3)

	configWithInvalidTimeout := map[string]string{
		"json-key":        "$.value",
		"scheme":          scheme,
		"path":            path,
		"port":            port,
		"request-timeout": "-256ms",
	}
	_, err4 := NewPodMetricsJSONPathGetter(configWithInvalidTimeout)
	require.Error(t, err4)

	configWithInvalidTimeout = map[string]string{
		"json-key":        "$.value",
		"scheme":          scheme,
		"path":            path,
		"port":            port,
		"connect-timeout": "-256ms",
	}
	_, err5 := NewPodMetricsJSONPathGetter(configWithInvalidTimeout)
	require.Error(t, err5)
}
