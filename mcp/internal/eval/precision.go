package eval

// PrecisionAtK returns the fraction of relevant results in the top-k.
// The denominator is always k, even when fewer than k results are available.
func PrecisionAtK(grades []int, k int, threshold int) float64 {
	if k <= 0 {
		return 0
	}
	relevant := 0
	for i := 0; i < k && i < len(grades); i++ {
		if grades[i] >= threshold {
			relevant++
		}
	}
	return float64(relevant) / float64(k)
}
