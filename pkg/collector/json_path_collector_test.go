package collector

import (
	"testing"

	"github.com/oliveagle/jsonpath"
	"github.com/stretchr/testify/require"
)

func TestNewJSONPathMetricsGetter(t *testing.T) {
	configNoAggregator := map[string]string{
		"json-key": "$.value",
		"scheme":   "http",
		"path":     "/metrics",
		"port":     "9090",
	}
	jpath1, _ := jsonpath.Compile(configNoAggregator["json-key"])
	getterNoAggregator, err1 := NewJSONPathMetricsGetter(configNoAggregator)

	require.NoError(t, err1)
	require.Equal(t, &JSONPathMetricsGetter{
		jsonPath: jpath1,
		scheme:   "http",
		path:     "/metrics",
		port:     9090,
	}, getterNoAggregator)

	configAggregator := map[string]string{
		"json-key":   "$.values",
		"scheme":     "http",
		"path":       "/metrics",
		"port":       "9090",
		"aggregator": "avg",
	}
	jpath2, _ := jsonpath.Compile(configAggregator["json-key"])
	getterAggregator, err2 := NewJSONPathMetricsGetter(configAggregator)

	require.NoError(t, err2)
	require.Equal(t, &JSONPathMetricsGetter{
		jsonPath:   jpath2,
		scheme:     "http",
		path:       "/metrics",
		port:       9090,
		aggregator: "avg",
	}, getterAggregator)

	configErrorJSONPath := map[string]string{
		"json-key": "{}",
		"scheme":   "http",
		"path":     "/metrics",
		"port":     "9090",
	}

	_, err3 := NewJSONPathMetricsGetter(configErrorJSONPath)
	require.Error(t, err3)

	configErrorPort := map[string]string{
		"json-key": "$.values",
		"scheme":   "http",
		"path":     "/metrics",
		"port":     "a9090",
	}

	_, err4 := NewJSONPathMetricsGetter(configErrorPort)
	require.Error(t, err4)
}

func TestIntsToFloat64s(t *testing.T) {
	noInts := []int{}
	noFloat64s := intsToFloat64s(noInts)
	require.Equal(t, []float64{}, noFloat64s)

	someInts := []int{1, 2, 3}
	someFloat64s := intsToFloat64s(someInts)
	require.Equal(t, []float64{1.0, 2.0, 3.0}, someFloat64s)
}

func TestFloat32sToFloat64s(t *testing.T) {
	noFloat32s := []float32{}
	noFloat64s := float32sToFloat64s(noFloat32s)
	require.Equal(t, []float64{}, noFloat64s)

	someFloat32s := []float32{1.0, 2.0, 3.0}
	someFloat64s := float32sToFloat64s(someFloat32s)
	require.Equal(t, []float64{1.0, 2.0, 3.0}, someFloat64s)
}

func TestReduce(t *testing.T) {
	average, err1 := reduce([]float64{1, 2, 3}, "avg")
	require.NoError(t, err1)
	require.Equal(t, 2.0, average)

	min, err2 := reduce([]float64{1, 2, 3}, "min")
	require.NoError(t, err2)
	require.Equal(t, 1.0, min)

	max, err3 := reduce([]float64{1, 2, 3}, "max")
	require.NoError(t, err3)
	require.Equal(t, 3.0, max)

	sum, err4 := reduce([]float64{1, 2, 3}, "sum")
	require.NoError(t, err4)
	require.Equal(t, 6.0, sum)

	_, err5 := reduce([]float64{1, 2, 3}, "inexistent_function")
	require.Errorf(t, err5, "slice of numbers was returned by JSONPath, but no valid aggregator function was specified: %v", "inexistent_function")
}
