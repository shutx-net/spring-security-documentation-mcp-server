package eval

import (
	"math"
	"sort"
)

// DCG computes Discounted Cumulative Gain at k.
// grades[i] is the relevance grade at rank i+1 (0-indexed).
func DCG(grades []int, k int) float64 {
	dcg := 0.0
	for i := 0; i < k && i < len(grades); i++ {
		dcg += (math.Pow(2, float64(grades[i])) - 1) / math.Log2(float64(i+2))
	}
	return dcg
}

// IdealGrades returns grades sorted in descending order (ideal ranking).
func IdealGrades(grades []int) []int {
	sorted := make([]int, len(grades))
	copy(sorted, grades)
	sort.Sort(sort.Reverse(sort.IntSlice(sorted)))
	return sorted
}

// NDCG computes nDCG@k. Returns 0 when IDCG is 0.
func NDCG(resultGrades []int, qrelGrades []int, k int) float64 {
	idcg := DCG(IdealGrades(qrelGrades), k)
	if idcg == 0 {
		return 0
	}
	return DCG(resultGrades, k) / idcg
}
