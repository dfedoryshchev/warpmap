package metrics

import (
	"path/filepath"
	"sort"
)

type Hotspot struct {
	File       string
	Churn      int
	Complexity int
	Score      float64
}

// Hotspots ranks files by normalized churn x complexity: the files that change
// often AND are hard to change. files are absolute paths; churn is keyed
// repo-relative, so each is relativized before the lookup.
func Hotspots(repoDir string, files []string, churn Churn) []Hotspot {
	rows := make([]Hotspot, 0, len(files))
	maxChurn, maxCx := 1, 1
	for _, f := range files {
		rel, _ := filepath.Rel(repoDir, f)
		c := churn[rel]
		cx, _ := FileComplexity(f)
		if c > maxChurn {
			maxChurn = c
		}
		if cx.Score > maxCx {
			maxCx = cx.Score
		}
		rows = append(rows, Hotspot{File: f, Churn: c, Complexity: cx.Score})
	}
	for i := range rows {
		rows[i].Score = (float64(rows[i].Churn) / float64(maxChurn)) *
			(float64(rows[i].Complexity) / float64(maxCx))
	}
	sort.Slice(rows, func(i, j int) bool { return rows[i].Score > rows[j].Score })
	return rows
}
