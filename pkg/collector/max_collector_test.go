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
	if c.value > 0 {
		quantity := resource.NewQuantity(c.value, resource.DecimalSI)
		return []CollectedMetric{
			{
				Custom: custom_metrics.MetricValue{
					Value: *quantity,
				},
			},
		}, nil
	} else {
		return nil, fmt.Errorf("test error")
	}
}

func TestMaxCollector(t *testing.T) {
	for _, tc := range []struct {
		name     string
		values   []int64
		expected int
		weight   float64
	}{
		{
			name:     "basic",
			values:   []int64{100, 10, 9},
			expected: 100,
			weight:   1,
		},
		{
			name:     "weighted",
			values:   []int64{100, 10, 9},
			expected: 20,
			weight:   0.2,
		},
		{
			name:     "with error",
			values:   []int64{-1, 10, 9},
			expected: 5,
			weight:   0.5,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			collectors := make([]Collector, len(tc.values))
			for i, v := range tc.values {
				collectors[i] = dummyCollector{value: v}
			}
			wc := NewMaxWeightedCollector(time.Second, tc.weight, collectors...)
			metrics, err := wc.GetMetrics()
			require.NoError(t, err)
			require.Len(t, metrics, 1)
			require.EqualValues(t, tc.expected, metrics[0].Custom.Value.Value())

		})

	}
}
