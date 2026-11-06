package baseline

import (
	"encoding/json"
	"fmt"
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
	// Functions is nil when no per-function data was taken (lizard absent, or a
	// snapshot written before the field existed) and a list, maybe empty, otherwise.
	Functions []Function `json:"functions"`
}

// Function is one function's measurements, keyed by its repo-relative file and name.
type Function struct {
	File   string `json:"file"`
	Name   string `json:"name"`
	NLOC   int    `json:"nloc"`
	CCN    int    `json:"ccn"`
	Params int    `json:"params"`
	Length int    `json:"length"`
}

// CaptureFunctions measures every function in files with the lizard executable at
// bin, named relative to dir.
func CaptureFunctions(bin, dir string, files []string) ([]Function, error) {
	fns, err := metrics.LizardFunctions(bin, files)
	if err != nil {
		return nil, err
	}
	out := make([]Function, 0, len(fns))
	for _, fn := range fns {
		rel, e := filepath.Rel(dir, fn.File)
		if e != nil {
			rel = fn.File
		}
		out = append(out, Function{File: filepath.ToSlash(rel), Name: fn.Name, NLOC: fn.NLOC, CCN: fn.CCN, Params: fn.Params, Length: fn.Length})
	}
	return out, nil
}

// FunctionChange is a function that is new since the baseline (Before nil) or
// grew in length, arguments or complexity.
type FunctionChange struct {
	Before *Function
	After  Function
}

// CompareFunctions lists the functions in after that are new or grew since before,
// most complex first.
func CompareFunctions(before, after []Function) []FunctionChange {
	// a file can hold several functions under one name (overloads, anonymous
	// callbacks), so the n-th of a name in a file pairs with the n-th one before.
	key := func(seen map[string]int, f Function) string {
		k := f.File + "\x00" + f.Name
		seen[k]++
		return fmt.Sprintf("%s\x00%d", k, seen[k])
	}
	old := map[string]Function{}
	seen := map[string]int{}
	for _, f := range before {
		old[key(seen, f)] = f
	}
	var out []FunctionChange
	seen = map[string]int{}
	for _, f := range after {
		b, ok := old[key(seen, f)]
		switch {
		case !ok:
			out = append(out, FunctionChange{After: f})
		case f.Length > b.Length || f.Params > b.Params || f.CCN > b.CCN:
			out = append(out, FunctionChange{Before: &b, After: f})
		}
	}
	sort.SliceStable(out, func(i, j int) bool {
		a, b := out[i].After, out[j].After
		if a.CCN != b.CCN {
			return a.CCN > b.CCN
		}
		if a.File != b.File {
			return a.File < b.File
		}
		return a.Name < b.Name
	})
	return out
}

func godCount(g graph.Graph) int {
	n := 0
	for _, m := range graph.GodModules(g) {
		if m.IsGod() {
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
		b := before.Complexity[f]
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
