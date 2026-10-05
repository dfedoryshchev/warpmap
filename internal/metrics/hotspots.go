package metrics

import (
	"path/filepath"
	"sort"

	"github.com/dfedoryshchev/warpmap/internal/coverage"
)

type Hotspot struct {
	File       string
	Churn      int
	Complexity int
	Score      float64
}

// Hotspots ranks files by normalized churn x complexity: the files that change
// often AND are hard to change. files are absolute paths; churn is keyed
// repo-relative with forward slashes (as git reports), so each is relativized and
// slash-normalized before the lookup - on Windows filepath.Rel yields backslashes,
// which never matched git's keys and silently zeroed every churn count.
//
// Test files are dropped before the normalisation, not after: a suite churns with
// the code it covers and is often longer, so it would set the scale for the rest.
func Hotspots(repoDir string, files []string, churn Churn) []Hotspot {
	return rank(repoDir, shipped(repoDir, files), churn, func(f string) int {
		cx, _ := FileComplexity(f)
		return cx.Score
	})
}

func shipped(repoDir string, files []string) []string {
	out := make([]string, 0, len(files))
	for _, f := range files {
		rel, _ := filepath.Rel(repoDir, f)
		if !coverage.IsTest(rel) {
			out = append(out, f)
		}
	}
	return out
}

func rank(repoDir string, files []string, churn Churn, complexity func(string) int) []Hotspot {
	rows := make([]Hotspot, 0, len(files))
	maxChurn, maxCx := 1, 1
	for _, f := range files {
		rel, _ := filepath.Rel(repoDir, f)
		c := churn[filepath.ToSlash(rel)]
		cx := complexity(f)
		if c > maxChurn {
			maxChurn = c
		}
		if cx > maxCx {
			maxCx = cx
		}
		rows = append(rows, Hotspot{File: f, Churn: c, Complexity: cx})
	}
	for i := range rows {
		rows[i].Score = (float64(rows[i].Churn) / float64(maxChurn)) *
			(float64(rows[i].Complexity) / float64(maxCx))
	}
	sort.Slice(rows, func(i, j int) bool { return rows[i].Score > rows[j].Score })
	return rows
}
