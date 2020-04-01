package httpmetrics

import (
	"fmt"
	"math"
)

type AggregatorFunc func(...float64) float64

// Average implements the average mathematical function over a slice of float64
func Average(values ...float64) float64 {
	sum := Sum(values...)
	return sum / float64(len(values))
}

// Minimum implements the absolute minimum mathematical function over a slice of float64
func Minimum(values ...float64) float64 {
	// initialized with positive infinity, all finite numbers are smaller than it
	curMin := math.Inf(1)
	for _, v := range values {
		if v < curMin {
			curMin = v
		}
	}
	return curMin
}

// Maximum implements the absolute maximum mathematical function over a slice of float64
func Maximum(values ...float64) float64 {
	// initialized with negative infinity, all finite numbers are bigger than it
	curMax := math.Inf(-1)
	for _, v := range values {
		if v > curMax {
			curMax = v
		}
	}
	return curMax
}

// Sum implements the summation mathematical function over a slice of float64
func Sum(values ...float64) float64 {
	res := 0.0

	for _, v := range values {
		res += v
	}

	return res
}

// reduce will reduce a slice of numbers given a aggregator function's name. If it's empty or not recognized, an error is returned.
func ParseAggregator(aggregator string) (AggregatorFunc, error) {
	switch aggregator {
	case "avg":
		return Average, nil
	case "min":
		return Minimum, nil
	case "max":
		return Maximum, nil
	case "sum":
		return Sum, nil
	default:
		return nil, fmt.Errorf("aggregator function: %s is unknown", aggregator)
	}
}
