package eval

import (
	"math"
	"testing"
)

func TestDCG_basic(t *testing.T) {
	grades := []int{3, 2, 1}
	want := 7.0/math.Log2(2) + 3.0/math.Log2(3) + 1.0/math.Log2(4)
	got := DCG(grades, 3)
	if math.Abs(got-want) > 1e-9 {
		t.Errorf("DCG = %f, want %f", got, want)
	}
}

func TestDCG_kLargerThanGrades(t *testing.T) {
	grades := []int{3}
	want := 7.0 / math.Log2(2)
	got := DCG(grades, 10)
	if math.Abs(got-want) > 1e-9 {
		t.Errorf("DCG = %f, want %f", got, want)
	}
}

func TestIdealGrades_sorting(t *testing.T) {
	got := IdealGrades([]int{1, 3, 2})
	if got[0] != 3 || got[1] != 2 || got[2] != 1 {
		t.Errorf("IdealGrades = %v, want [3 2 1]", got)
	}
}

func TestNDCG_perfectRanking(t *testing.T) {
	grades := []int{3, 2, 1}
	v := NDCG(grades, grades, 5)
	if math.Abs(v-1.0) > 1e-9 {
		t.Errorf("NDCG = %f, want 1.0", v)
	}
}

func TestNDCG_zeroIDCG(t *testing.T) {
	// All qrel grades 0 → IDCG = 0 → must return 0
	v := NDCG([]int{0}, []int{0}, 5)
	if v != 0 {
		t.Errorf("NDCG = %f, want 0.0", v)
	}
}

func TestNDCG_lowerRankingDecreasesScore(t *testing.T) {
	qrelGrades := []int{3, 2}
	perfect := NDCG([]int{3, 2}, qrelGrades, 5)
	degraded := NDCG([]int{2, 3}, qrelGrades, 5)
	if perfect <= degraded {
		t.Errorf("perfect nDCG (%f) should be > degraded nDCG (%f)", perfect, degraded)
	}
}

func TestNDCG_kLargerThanResults(t *testing.T) {
	// k=10, only 1 result — should not panic
	v := NDCG([]int{3}, []int{3}, 10)
	if math.Abs(v-1.0) > 1e-9 {
		t.Errorf("NDCG = %f, want 1.0", v)
	}
}
