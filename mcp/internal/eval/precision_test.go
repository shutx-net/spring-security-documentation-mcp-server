package eval

import (
	"math"
	"testing"
)

func TestPrecisionAtK_basic(t *testing.T) {
	// grades: [3, 2, 1, 0, 3] → relevant at threshold=2: indices 0,1,4 → 3/5
	grades := []int{3, 2, 1, 0, 3}
	got := PrecisionAtK(grades, 5, 2)
	want := 0.6
	if math.Abs(got-want) > 1e-9 {
		t.Errorf("got %f, want %f", got, want)
	}
}

func TestPrecisionAtK_shortResultsUsesKAsDenominator(t *testing.T) {
	// 2 results, k=5: 2 relevant / 5
	grades := []int{3, 2}
	got := PrecisionAtK(grades, 5, 2)
	want := 0.4
	if math.Abs(got-want) > 1e-9 {
		t.Errorf("got %f, want %f", got, want)
	}
}

func TestPrecisionAtK_strictThreshold(t *testing.T) {
	// threshold=3: only grade 3 is relevant → 1/5
	grades := []int{3, 2, 2, 1, 0}
	got := PrecisionAtK(grades, 5, 3)
	want := 0.2
	if math.Abs(got-want) > 1e-9 {
		t.Errorf("got %f, want %f", got, want)
	}
}

func TestPrecisionAtK_allRelevant(t *testing.T) {
	grades := []int{3, 3, 3, 2, 2}
	got := PrecisionAtK(grades, 5, 2)
	if math.Abs(got-1.0) > 1e-9 {
		t.Errorf("got %f, want 1.0", got)
	}
}

func TestPrecisionAtK_noneRelevant(t *testing.T) {
	grades := []int{1, 0, 1, 0, 1}
	got := PrecisionAtK(grades, 5, 2)
	if math.Abs(got-0.0) > 1e-9 {
		t.Errorf("got %f, want 0.0", got)
	}
}

func TestPrecisionAtK_emptyGrades(t *testing.T) {
	got := PrecisionAtK(nil, 5, 2)
	if math.Abs(got-0.0) > 1e-9 {
		t.Errorf("got %f, want 0.0", got)
	}
}
