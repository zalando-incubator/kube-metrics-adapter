package collector

import (
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"k8s.io/apimachinery/pkg/api/resource"
	"k8s.io/metrics/pkg/apis/custom_metrics"
)

type dummyCollector struct {
	value int64
}

func (c dummyCollector) Interval() time.Duration {
	return time.Second
}

func (c dummyCollector) GetMetrics() ([]CollectedMetric, error) {
	switch c.value {
	case 0:
		return nil, NoResultError{query: "invalid query"}
	case -1:
		return nil, fmt.Errorf("test error")
	default:
		quantity := resource.NewQuantity(c.value, resource.DecimalSI)
		return []CollectedMetric{
			{
				Custom: custom_metrics.MetricValue{
					Value: *quantity,
				},
			},
		}, nil
	}
}

func TestMaxCollector(t *testing.T) {
	for _, tc := range []struct {
		name     string
		values   []int64
		expected int
		weight   float64
		errored  bool
	}{
		{
			name:     "basic",
			values:   []int64{100, 10, 9},
			expected: 100,
			weight:   1,
			errored:  false,
		},
		{
			name:     "weighted",
			values:   []int64{100, 10, 9},
			expected: 20,
			weight:   0.2,
			errored:  false,
		},
		{
			name:    "with error",
			values:  []int64{10, 9, -1},
			weight:  0.5,
			errored: true,
		},
		{
			name:     "some invalid results",
			values:   []int64{0, 1, 0, 10, 9},
			expected: 5,
			weight:   0.5,
			errored:  false,
		},
		{
			name:    "both invalid results and errors",
			values:  []int64{0, 1, 0, -1, 10, 9},
			weight:  0.5,
			errored: true,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			collectors := make([]Collector, len(tc.values))
			for i, v := range tc.values {
				collectors[i] = dummyCollector{value: v}
			}
			wc := NewMaxWeightedCollector(time.Second, tc.weight, collectors...)
			metrics, err := wc.GetMetrics()
			if tc.errored {
				require.Error(t, err)
			} else {
				require.NoError(t, err)
				require.Len(t, metrics, 1)
				require.EqualValues(t, tc.expected, metrics[0].Custom.Value.Value())
			}

		})

	}
}
