package httpmetrics

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"

	"github.com/oliveagle/jsonpath"
	"github.com/stretchr/testify/require"
)

func TestCastSlice(t *testing.T) {
	res1, err1 := castSlice([]interface{}{1, 2, 3})
	require.NoError(t, err1)
	require.Equal(t, []float64{1.0, 2.0, 3.0}, res1)

	res2, err2 := castSlice([]interface{}{float32(1.0), float32(2.0), float32(3.0)})
	require.NoError(t, err2)
	require.Equal(t, []float64{1.0, 2.0, 3.0}, res2)

	res3, err3 := castSlice([]interface{}{float64(1.0), float64(2.0), float64(3.0)})
	require.NoError(t, err3)
	require.Equal(t, []float64{1.0, 2.0, 3.0}, res3)

	res4, err4 := castSlice([]interface{}{1, 2, "some string"})
	require.Errorf(t, err4, "slice was returned by JSONPath, but value inside is unsupported: %T", "string")
	require.Equal(t, []float64(nil), res4)
}

type testValueResponse struct {
	Value int64 `json:"value"`
}

type testValueArrayResponse struct {
	Value []int64 `json:"value"`
}

func makeTestHTTPServer(t *testing.T, values ...int64) *httptest.Server {
	h := func(w http.ResponseWriter, r *http.Request) {
		require.Equal(t, r.URL.Path, "/metrics")
		w.Header().Set("Content-Type", "application/json")
		var (
			response []byte
			err      error
		)
		if len(values) == 1 {
			response, err = json.Marshal(testValueResponse{Value: values[0]})
			require.NoError(t, err)
		} else {
			response, err = json.Marshal(testValueArrayResponse{Value: values})
			require.NoError(t, err)
		}
		_, err = w.Write(response)
		require.NoError(t, err)
	}
	return httptest.NewServer(http.HandlerFunc(h))
}

func TestJSONPathMetricsGetter(t *testing.T) {
	for _, tc := range []struct {
		name       string
		input      []int64
		output     float64
		aggregator AggregatorFunc
	}{
		{
			name:       "basic average",
			input:      []int64{3, 4, 5},
			output:     4,
			aggregator: Average,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			server := makeTestHTTPServer(t, tc.input...)
			defer server.Close()
			path, err := jsonpath.Compile("$.value")
			require.NoError(t, err)
			getter := NewJSONPathMetricsGetter(DefaultMetricsHTTPClient(), tc.aggregator, path)
			url, err := url.Parse(fmt.Sprintf("%s/metrics", server.URL))
			require.NoError(t, err)
			metric, err := getter.GetMetric(*url)
			require.NoError(t, err)
			require.Equal(t, tc.output, metric)
		})
	}
}
