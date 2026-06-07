package eval

import "testing"

func TestRunReportToMarkdown(t *testing.T) {
	report := &RunReport{
		RunID:      "hybrid",
		TopicCount: 2,
		Metrics: map[string]float64{
			"ndcg@5":       0.8234,
			"ndcg@10":      0.7912,
			"precision@5":  0.6667,
			"precision@10": 0.52,
		},
	}

	got := RunReportToMarkdown(report)

	want := `## Evaluation: hybrid

Topics: 2

| k | nDCG@k | Precision@k |
| --- | --- | --- |
| 5 | 0.823 | 0.667 |
| 10 | 0.791 | 0.520 |
`
	if got != want {
		t.Errorf("RunReportToMarkdown() =\n%s\nwant:\n%s", got, want)
	}
}

func TestRunReportToMarkdown_emptyMetrics(t *testing.T) {
	report := &RunReport{RunID: "hybrid", TopicCount: 0, Metrics: map[string]float64{}}

	got := RunReportToMarkdown(report)

	want := `## Evaluation: hybrid

Topics: 0

| k | nDCG@k | Precision@k |
| --- | --- | --- |
`
	if got != want {
		t.Errorf("RunReportToMarkdown() =\n%s\nwant:\n%s", got, want)
	}
}
