package baseline

import (
	"encoding/json"
	"os"
	"path/filepath"
	"sort"

	"github.com/dfedoryshchev/warpmap/internal/graph"
	"github.com/dfedoryshchev/warpmap/internal/metrics"
)

// Snapshot is warpmap's "hard memory" - where the codebase was, so a later run can
// tell whether a change made it better or worse.
type Snapshot struct {
	Files      int            `json:"files"`
	Edges      int            `json:"edges"`
	Cycles     int            `json:"cycles"`
	Orphans    int            `json:"orphans"`
	GodModules int            `json:"godModules"`
	Complexity map[string]int `json:"complexity"` // repo-relative path -> score
}

func godCount(g graph.Graph) int {
	n := 0
	for _, m := range graph.GodModules(g) {
		if m.FanIn+m.FanOut >= 20 {
			n++
		}
	}
	return n
}

// Capture computes a snapshot of the current state.
func Capture(dir string, files []string) Snapshot {
	g := graph.Build(files)
	s := Snapshot{
		Files:      len(g.Files),
		Edges:      len(g.Edges),
		Cycles:     len(graph.Cycles(g)),
		Orphans:    len(graph.Orphans(g)),
		GodModules: godCount(g),
		Complexity: map[string]int{},
	}
	for _, f := range files {
		c, err := metrics.FileComplexity(f)
		if err != nil {
			continue
		}
		rel, e := filepath.Rel(dir, f)
		if e != nil {
			rel = f
		}
		s.Complexity[filepath.ToSlash(rel)] = c.Score
	}
	return s
}

func Path(dir string) string { return filepath.Join(dir, ".warpmap", "baseline.json") }

func Save(dir string, s Snapshot) error {
	if err := os.MkdirAll(filepath.Join(dir, ".warpmap"), 0o755); err != nil {
		return err
	}
	data, err := json.MarshalIndent(s, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(Path(dir), data, 0o644)
}

func Load(dir string) (Snapshot, error) {
	var s Snapshot
	data, err := os.ReadFile(Path(dir))
	if err != nil {
		return s, err
	}
	return s, json.Unmarshal(data, &s)
}

type Change struct {
	File   string
	Before int
	After  int
}

// Diff is the ratchet: how the codebase moved between the baseline and now.
type Diff struct {
	Edges      int // deltas (after - before); positive = grew
	Cycles     int
	Orphans    int
	GodModules int
	Worsened   []Change // files whose complexity went up
	Improved   []Change // files whose complexity went down
}

// RiskUp reports whether the change made the codebase harder to work on safely.
func (d Diff) RiskUp() bool {
	return d.Cycles > 0 || d.GodModules > 0 || len(d.Worsened) > len(d.Improved)
}

func Compare(before, after Snapshot) Diff {
	d := Diff{
		Edges:      after.Edges - before.Edges,
		Cycles:     after.Cycles - before.Cycles,
		Orphans:    after.Orphans - before.Orphans,
		GodModules: after.GodModules - before.GodModules,
	}
	for f, a := range after.Complexity {
		b, ok := before.Complexity[f]
		if !ok {
			continue // a file with no baseline is not a regression
		}
		switch {
		case a > b:
			d.Worsened = append(d.Worsened, Change{f, b, a})
		case a < b:
			d.Improved = append(d.Improved, Change{f, b, a})
		}
	}
	sort.Slice(d.Worsened, func(i, j int) bool {
		return d.Worsened[i].After-d.Worsened[i].Before > d.Worsened[j].After-d.Worsened[j].Before
	})
	sort.Slice(d.Improved, func(i, j int) bool {
		return d.Improved[i].Before-d.Improved[i].After > d.Improved[j].Before-d.Improved[j].After
	})
	return d
}
