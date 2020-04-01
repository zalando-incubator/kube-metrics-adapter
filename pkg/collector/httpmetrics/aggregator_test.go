package httpmetrics

import (
	"fmt"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestReduce(t *testing.T) {
	for _, tc := range []struct {
		input      []float64
		output     float64
		aggregator string
		parseError bool
	}{
		{
			input:      []float64{1, 2, 3},
			output:     2.0,
			aggregator: "avg",
			parseError: false,
		},
		{
			input:      []float64{1, 2, 3},
			output:     1.0,
			aggregator: "min",
			parseError: false,
		},
		{
			input:      []float64{1, 2, 3},
			output:     3.0,
			aggregator: "max",
			parseError: false,
		},
		{
			input:      []float64{1, 2, 3},
			output:     6.0,
			aggregator: "sum",
			parseError: false,
		},
		{
			input:      []float64{1, 2, 3},
			aggregator: "non-existent",
			parseError: true,
		},
	} {
		t.Run(fmt.Sprintf("Test function: %s", tc.aggregator), func(t *testing.T) {
			aggFunc, err := ParseAggregator(tc.aggregator)
			if tc.parseError {
				require.Error(t, err)
			} else {
				val := aggFunc(tc.input...)
				require.Equal(t, tc.output, val)
			}
		})
	}
}
