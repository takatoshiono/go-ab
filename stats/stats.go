package stats

import (
	"math"
	"sort"
)

func Min(values []float64) float64 {
	var min = math.MaxFloat64
	for _, v := range values {
		min = math.Min(min, v)
	}
	return min
}

func Max(values []float64) float64 {
	var max float64
	for _, v := range values {
		max = math.Max(max, v)
	}
	return max
}

func Sum(values []float64) float64 {
	var sum float64
	for _, v := range values {
		sum += v
	}
	return sum
}

func Mean(values []float64) float64 {
	return Sum(values) / float64(len(values))
}

func StandardDeviation(values []float64) float64 {
	var n = len(values)
	var variance float64

	mean := Mean(values)

	for _, v := range values {
		variance += math.Pow((v - mean), 2)
	}

	if n > 1 {
		return math.Sqrt(variance / float64(n-1))
	} else {
		return 0
	}
}

func Median(values []float64) float64 {
	var n = len(values)
	c := make([]float64, n)
	copy(c, values)
	sort.Float64s(c)
	if n > 1 && (n%2) != 0 {
		return (c[n/2] + c[n/2+1]) / 2
	} else {
		return c[n/2]
	}
}
