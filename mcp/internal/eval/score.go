package eval

import (
	"fmt"
	"sort"
	"strconv"
)

// ScoreRun computes nDCG metrics for a run against relevance judgments.
// Topics are defined by qrels; run entries for topics absent from qrels are ignored.
func ScoreRun(qrels []Qrel, runEntries []RunEntry, opts ScoreOptions) (*RunReport, error) {
	ks := opts.Ks
	if len(ks) == 0 {
		ks = []int{5, 10}
	}

	// Build qrel lookup: topicId -> chunkId -> grade
	qrelMap := make(map[string]map[string]int)
	for _, q := range qrels {
		if qrelMap[q.TopicID] == nil {
			qrelMap[q.TopicID] = make(map[string]int)
		}
		qrelMap[q.TopicID][q.ChunkID] = q.Grade
	}

	// Group and validate run entries by topic.
	// Topics absent from qrels are skipped early (fix 8).
	type topicRun struct {
		entries   []RunEntry
		seenChunk map[string]struct{}
		seenRank  map[int]struct{}
	}
	byTopic := make(map[string]*topicRun)
	var runID string
	for _, e := range runEntries {
		if runID == "" {
			runID = e.RunID
		}
		if qrelMap[e.TopicID] == nil {
			continue // fix 8: skip topics not in qrels
		}
		if byTopic[e.TopicID] == nil {
			byTopic[e.TopicID] = &topicRun{
				seenChunk: make(map[string]struct{}),
				seenRank:  make(map[int]struct{}),
			}
		}
		tr := byTopic[e.TopicID]
		if _, dup := tr.seenRank[e.Rank]; dup {
			return nil, fmt.Errorf("duplicate rank %d in topicId %q", e.Rank, e.TopicID) // fix 1
		}
		tr.seenRank[e.Rank] = struct{}{}
		if _, dup := tr.seenChunk[e.ChunkID]; dup {
			return nil, fmt.Errorf("duplicate chunkId %q in topicId %q", e.ChunkID, e.TopicID)
		}
		tr.seenChunk[e.ChunkID] = struct{}{}
		tr.entries = append(tr.entries, e)
	}
	for _, tr := range byTopic {
		sort.Slice(tr.entries, func(i, j int) bool {
			return tr.entries[i].Rank < tr.entries[j].Rank
		})
	}

	// Topics are defined by qrels
	topicIDs := make([]string, 0, len(qrelMap))
	for tid := range qrelMap {
		topicIDs = append(topicIDs, tid)
	}
	sort.Strings(topicIDs)

	sumNDCG := make(map[int]float64)
	sumPrecision := make(map[int]float64)
	sumUnjudged := make(map[int]int)
	topics := []TopicMetric{} // fix 4: empty slice so JSON encodes as [] not null
	threshold := opts.relevanceThreshold()

	for _, tid := range topicIDs {
		chunkGrades := qrelMap[tid]

		// Ideal grades from all qrels for this topic, sorted once (fix 6)
		idealG := make([]int, 0, len(chunkGrades))
		for _, g := range chunkGrades {
			idealG = append(idealG, g)
		}
		idealSorted := IdealGrades(idealG)

		// Result grades from run; unjudged@k uses array index to match DCG cutoff (fix 2)
		var resultGrades []int
		perTopicUnjudged := make(map[int]int)
		if tr, ok := byTopic[tid]; ok {
			for idx, e := range tr.entries {
				g, judged := chunkGrades[e.ChunkID]
				if !judged {
					g = 0
					for _, k := range ks {
						if idx < k { // fix 2: array index, consistent with DCG i < k
							perTopicUnjudged[k]++
						}
					}
				}
				resultGrades = append(resultGrades, g)
			}
		}

		topicNDCG := make(map[string]float64)
		topicPrecision := make(map[string]float64)
		for _, k := range ks {
			ndcgKey := fmt.Sprintf("ndcg@%d", k)
			// fix 6: use pre-sorted ideal grades, no repeated sort
			idcg := DCG(idealSorted, k)
			var v float64
			if idcg > 0 {
				v = DCG(resultGrades, k) / idcg
			}
			topicNDCG[ndcgKey] = v
			sumNDCG[k] += v
			sumUnjudged[k] += perTopicUnjudged[k]

			precisionKey := fmt.Sprintf("precision@%d", k)
			p := PrecisionAtK(resultGrades, k, threshold)
			topicPrecision[precisionKey] = p
			sumPrecision[k] += p
		}

		unjudgedStr := make(map[string]int, len(ks))
		for _, k := range ks {
			unjudgedStr[strconv.Itoa(k)] = perTopicUnjudged[k]
		}

		topics = append(topics, TopicMetric{
			TopicID:     tid,
			NDCG:        topicNDCG,
			Precision:   topicPrecision,
			UnjudgedAtK: unjudgedStr,
		})
	}

	n := len(topicIDs)
	metrics := make(map[string]float64, len(ks)*2)
	meanUnjudged := make(map[string]float64, len(ks))
	for _, k := range ks {
		if n > 0 {
			metrics[fmt.Sprintf("ndcg@%d", k)] = sumNDCG[k] / float64(n)
			metrics[fmt.Sprintf("precision@%d", k)] = sumPrecision[k] / float64(n)
			meanUnjudged[strconv.Itoa(k)] = float64(sumUnjudged[k]) / float64(n)
		}
	}

	return &RunReport{
		RunID:       runID,
		Metrics:     metrics,
		TopicCount:  n,
		Topics:      topics,
		UnjudgedAtK: meanUnjudged,
	}, nil
}
