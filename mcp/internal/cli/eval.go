package cli

import (
	"fmt"
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
	cmd.AddCommand(newEvalScoreCmd())
	return cmd
}

func newEvalScoreCmd() *cobra.Command {
	var (
		qrelsPath  string
		runPath    string
		kStr       string
		outputPath string
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

			report, err := eval.ScoreRun(qrels, runEntries, eval.ScoreOptions{Ks: ks})
			if err != nil {
				return fmt.Errorf("score run: %w", err)
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

			return eval.WriteJSONReport(out, report)
		},
	}

	cmd.Flags().StringVar(&qrelsPath, "qrels", "", "qrels JSONL file path [required]")
	cmd.Flags().StringVar(&runPath, "run", "", "run JSONL file path [required]")
	cmd.Flags().StringVar(&kStr, "k", "5,10", "comma-separated k values")
	cmd.Flags().StringVar(&outputPath, "output", "", "output JSON report path (stdout if omitted)")
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
