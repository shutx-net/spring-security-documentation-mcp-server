package eval

import (
	"encoding/json"
	"fmt"
	"io"
	"sort"
	"strconv"
	"strings"
)

// WriteJSONReport encodes report as indented JSON to w.
func WriteJSONReport(w io.Writer, report *RunReport) error {
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	return enc.Encode(report)
}

// RunReportToMarkdown renders report's nDCG@k / Precision@k metrics as a
// Markdown table, e.g. for display in a GitHub Actions step summary.
func RunReportToMarkdown(report *RunReport) string {
	var b strings.Builder
	fmt.Fprintf(&b, "## Evaluation: %s\n\n", report.RunID)
	fmt.Fprintf(&b, "Topics: %d\n\n", report.TopicCount)
	b.WriteString("| k | nDCG@k | Precision@k |\n")
	b.WriteString("| --- | --- | --- |\n")
	for _, k := range sortedMetricKs(report.Metrics) {
		fmt.Fprintf(&b, "| %d | %.3f | %.3f |\n",
			k,
			report.Metrics[fmt.Sprintf("ndcg@%d", k)],
			report.Metrics[fmt.Sprintf("precision@%d", k)],
		)
	}
	return b.String()
}

// sortedMetricKs extracts the distinct k values from metrics keys formatted as
// "ndcg@<k>" / "precision@<k>" and returns them in ascending order. RunReport
// does not retain ScoreOptions.Ks, so the metric keys are the only source.
func sortedMetricKs(metrics map[string]float64) []int {
	seen := make(map[int]struct{})
	for key := range metrics {
		_, kStr, ok := strings.Cut(key, "@")
		if !ok {
			continue
		}
		k, err := strconv.Atoi(kStr)
		if err != nil {
			continue
		}
		seen[k] = struct{}{}
	}
	ks := make([]int, 0, len(seen))
	for k := range seen {
		ks = append(ks, k)
	}
	sort.Ints(ks)
	return ks
}
