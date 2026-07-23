package cli

import (
	"fmt"
	"log"
	"os"
	"strconv"
	"strings"

	"github.com/shutx-net/spring-security-documentation-mcp-server/internal/eval"
	"github.com/spf13/cobra"
)

func newEvalCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "eval",
		Short: "Evaluate search quality",
	}
	cmd.AddCommand(newEvalRunCmd())
	cmd.AddCommand(newEvalScoreCmd())
	return cmd
}

func newEvalRunCmd() *cobra.Command {
	var (
		topicsPath  string
		runID       string
		limit       int
		outputPath  string
		failOnEmpty bool
	)

	cmd := &cobra.Command{
		Use:   "run",
		Short: "Run evaluation topics against the search index and produce a run file",
		Example: `  spring-security-docs-mcp eval run \
    --topics eval/samples/topics.jsonl \
    --run-id hybrid \
    --limit  10 \
    --output /tmp/eval/hybrid.run.jsonl`,
		RunE: func(cmd *cobra.Command, args []string) error {
			topicsFile, err := os.Open(topicsPath)
			if err != nil {
				return fmt.Errorf("open topics: %w", err)
			}
			defer topicsFile.Close()

			topics, err := eval.LoadTopics(topicsFile)
			if err != nil {
				return fmt.Errorf("load topics: %w", err)
			}

			st, err := openAWSStore(cmd.Context())
			if err != nil {
				return err
			}
			defer st.Close()

			log.Printf("Running %d topics against the search index (runId=%s, limit=%d)", len(topics), runID, limit)
			entries, err := eval.SearchTopics(cmd.Context(), st, topics, eval.RunOptions{
				RunID: runID,
				Limit: limit,
				OnTopic: func(i, total int, t eval.Topic) {
					log.Printf("[%d/%d] searching topic %q", i, total, t.TopicID)
				},
			})
			if err != nil {
				return fmt.Errorf("run topics: %w", err)
			}
			log.Printf("Done: %d run entries from %d topics (runId=%s)", len(entries), len(topics), runID)

			out := os.Stdout
			if outputPath != "" {
				f, err := os.Create(outputPath)
				if err != nil {
					return fmt.Errorf("create output: %w", err)
				}
				defer f.Close()
				out = f
			}
			if err := eval.WriteRun(out, entries); err != nil {
				return err
			}
			// Guardrail: an empty run means the search pipeline is broken
			// (e.g. an auth or filter error). Fail loudly instead of letting
			// a downstream score of nothing look "green".
			if failOnEmpty && len(entries) == 0 {
				return fmt.Errorf("no run entries produced from %d topics; the search pipeline is likely broken", len(topics))
			}
			return nil
		},
	}

	cmd.Flags().StringVar(&topicsPath, "topics", "", "topics JSONL file path [required]")
	cmd.Flags().StringVar(&runID, "run-id", "", "identifier recorded in each run entry [required]")
	cmd.Flags().IntVar(&limit, "limit", 10, "maximum number of results per topic (the search backend clamps values outside 1-20 to 10)")
	cmd.Flags().StringVar(&outputPath, "output", "", "output run JSONL path (stdout if omitted)")
	cmd.Flags().BoolVar(&failOnEmpty, "fail-on-empty", true, "exit non-zero when no run entries are produced (guards against a silently broken search pipeline)")
	_ = cmd.MarkFlagRequired("topics")
	_ = cmd.MarkFlagRequired("run-id")
	return cmd
}

func newEvalScoreCmd() *cobra.Command {
	var (
		qrelsPath          string
		runPath            string
		kStr               string
		outputPath         string
		markdownOutputPath string
		relevanceThreshold int
		failOnZeroJudged   bool
	)

	cmd := &cobra.Command{
		Use:   "score",
		Short: "Compute nDCG scores from qrels and a run file",
		Example: `  spring-security-docs-mcp eval score \
    --qrels eval/samples/qrels.jsonl \
    --run   eval/samples/hybrid.run.jsonl \
    --k     5,10`,
		RunE: func(cmd *cobra.Command, args []string) error {
			ks, err := parseKs(kStr)
			if err != nil {
				return fmt.Errorf("--k: %w", err)
			}

			qrelsFile, err := os.Open(qrelsPath)
			if err != nil {
				return fmt.Errorf("open qrels: %w", err)
			}
			defer qrelsFile.Close()

			runFile, err := os.Open(runPath)
			if err != nil {
				return fmt.Errorf("open run: %w", err)
			}
			defer runFile.Close()

			qrels, err := eval.LoadQrels(qrelsFile)
			if err != nil {
				return fmt.Errorf("load qrels: %w", err)
			}
			runEntries, err := eval.LoadRun(runFile)
			if err != nil {
				return fmt.Errorf("load run: %w", err)
			}

			report, err := eval.ScoreRun(qrels, runEntries, eval.ScoreOptions{Ks: ks, RelevanceThreshold: relevanceThreshold})
			if err != nil {
				return fmt.Errorf("score run: %w", err)
			}

			if markdownOutputPath != "" {
				f, err := os.Create(markdownOutputPath)
				if err != nil {
					return fmt.Errorf("create markdown output: %w", err)
				}
				defer f.Close()
				if _, err := f.WriteString(eval.RunReportToMarkdown(report)); err != nil {
					return fmt.Errorf("write markdown output: %w", err)
				}
			}

			out := os.Stdout
			if outputPath != "" {
				f, err := os.Create(outputPath)
				if err != nil {
					return fmt.Errorf("create output: %w", err)
				}
				defer f.Close()
				out = f
			}

			if err := eval.WriteJSONReport(out, report); err != nil {
				return err
			}
			// Guardrail: retrieving results none of which are even in the qrels
			// pool means the run and the fixture share no items — almost always
			// a stale qrels fixture (chunkIds changed on reindex). Fail loudly
			// so a 0.000 score is never mistaken for "green".
			if failOnZeroJudged && report.ScoredEntries > 0 && report.JudgedEntries == 0 {
				return fmt.Errorf("no retrieved chunk matched any qrel (0/%d judged); the qrels fixture is likely stale — regenerate it against the current index", report.ScoredEntries)
			}
			return nil
		},
	}

	cmd.Flags().StringVar(&qrelsPath, "qrels", "", "qrels JSONL file path [required]")
	cmd.Flags().StringVar(&runPath, "run", "", "run JSONL file path [required]")
	cmd.Flags().StringVar(&kStr, "k", "5,10", "comma-separated k values")
	cmd.Flags().StringVar(&outputPath, "output", "", "output JSON report path (stdout if omitted)")
	cmd.Flags().StringVar(&markdownOutputPath, "markdown-output", "", "write a Markdown summary table to this path (e.g. for GitHub Actions step summaries)")
	cmd.Flags().IntVar(&relevanceThreshold, "relevance-threshold", 2, "minimum qrel grade treated as relevant for binary metrics")
	cmd.Flags().BoolVar(&failOnZeroJudged, "fail-on-zero-judged", true, "exit non-zero when no retrieved chunk matches any qrel (guards against a stale fixture)")
	_ = cmd.MarkFlagRequired("qrels")
	_ = cmd.MarkFlagRequired("run")
	return cmd
}

func parseKs(s string) ([]int, error) {
	parts := strings.Split(s, ",")
	ks := make([]int, 0, len(parts))
	for _, p := range parts {
		p = strings.TrimSpace(p)
		k, err := strconv.Atoi(p)
		if err != nil || k < 1 {
			return nil, fmt.Errorf("invalid k value %q: must be a positive integer", p)
		}
		ks = append(ks, k)
	}
	return ks, nil
}
