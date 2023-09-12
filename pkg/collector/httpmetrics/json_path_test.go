package httpmetrics

import (
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"

	"github.com/stretchr/testify/require"
)

func makeTestHTTPServer(t *testing.T, response []byte) *httptest.Server {
	h := func(w http.ResponseWriter, r *http.Request) {
		require.Equal(t, r.URL.Path, "/metrics")
		w.Header().Set("Content-Type", "application/json")
		_, err := w.Write(response)
		require.NoError(t, err)
	}
	return httptest.NewServer(http.HandlerFunc(h))
}

func TestJSONPathMetricsGetter(t *testing.T) {
	for _, tc := range []struct {
		name         string
		jsonResponse []byte
		jsonPath     string
		result       float64
		aggregator   AggregatorFunc
		err          error
	}{
		{
			name:         "basic single value",
			jsonResponse: []byte(`{"value":3}`),
			jsonPath:     "$.value",
			result:       3,
			aggregator:   Average,
		},
		{
			name:         "basic average",
			jsonResponse: []byte(`{"value":[3,4,5]}`),
			jsonPath:     "$.value",
			result:       4,
			aggregator:   Average,
		},
		{
			name:         "dotted key",
			jsonResponse: []byte(`{"metric.value":5}`),
			jsonPath:     "$['metric.value']",
			result:       5,
			aggregator:   Average,
		},
		{
			name:         "glob array query",
			jsonResponse: []byte(`{"worker_status":[{"last_status":{"backlog":3}},{"last_status":{"backlog":7}}]}`),
			jsonPath:     "$.worker_status.[*].last_status.backlog",
			result:       5,
			aggregator:   Average,
		},
		{
			name:         "json path not resulting in array or number should lead to error",
			jsonResponse: []byte(`{"metric.value":5}`),
			jsonPath:     "$['invalid.metric.values']",
			err:          errors.New("unexpected json: expected single numeric or array value"),
		},
		{
			name:         "invalid json should error",
			jsonResponse: []byte(`{`),
			jsonPath:     "$['invalid.metric.values']",
			err:          errors.New("unexpected end of file"),
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			server := makeTestHTTPServer(t, tc.jsonResponse)
			defer server.Close()
			getter, err := NewJSONPathMetricsGetter(DefaultMetricsHTTPClient(), tc.aggregator, tc.jsonPath)
			require.NoError(t, err)
			url, err := url.Parse(fmt.Sprintf("%s/metrics", server.URL))
			require.NoError(t, err)
			metric, err := getter.GetMetric(*url)
			if tc.err != nil {
				require.Error(t, err)
				require.Equal(t, tc.err.Error(), err.Error())
				return
			}
			require.NoError(t, err)
			require.Equal(t, tc.result, metric)
		})
	}
}
