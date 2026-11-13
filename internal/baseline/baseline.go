package baseline

import (
	"encoding/json"
	"fmt"
	"maps"
	"os"
	"path/filepath"
	"slices"
	"sort"

	"github.com/dfedoryshchev/warpmap/internal/coverage"
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
	TestsGrew  []Change // test files whose complexity went up; never part of the verdict
}

// Term is one named condition of the verdict.
type Term struct {
	Name    string
	Tripped bool
}

// Terms lists every condition the verdict is made of, tripped or not. Each term
// carries its own exception: complexity leaves test files out, dead code is
// counted on net growth, and entry points (see graph.Orphans) never count as dead.
func (d Diff) Terms() []Term {
	return []Term{
		{"cycles", d.Cycles > 0},
		{"god-modules", d.GodModules > 0},
		{"complexity", len(d.Worsened) > len(d.Improved)},
		{"dead-code", d.Orphans > 0},
	}
}

// RiskUp reports whether the change made the codebase harder to work on safely:
// whether any term tripped.
func (d Diff) RiskUp() bool {
	for _, t := range d.Terms() {
		if t.Tripped {
			return true
		}
	}
	return false
}

func Compare(before, after Snapshot) Diff {
	d := Diff{
		Edges:      after.Edges - before.Edges,
		Cycles:     after.Cycles - before.Cycles,
		Orphans:    after.Orphans - before.Orphans,
		GodModules: after.GodModules - before.GodModules,
	}
	// the snapshot keys files by path, so a file moved unchanged would read as one
	// removed and one added; a removed file and an added one of the same kind and
	// score pair off as a move instead.
	type gone struct {
		test  bool
		score int
	}
	removed := map[gone]int{}
	for f, b := range before.Complexity {
		if _, ok := after.Complexity[f]; !ok {
			removed[gone{coverage.IsTest(f), b}]++
		}
	}
	for _, f := range slices.Sorted(maps.Keys(after.Complexity)) {
		a := after.Complexity[f]
		b, ok := before.Complexity[f]
		if k := (gone{coverage.IsTest(f), a}); !ok && removed[k] > 0 {
			removed[k]--
			continue
		}
		switch {
		case coverage.IsTest(f):
			if a > b {
				d.TestsGrew = append(d.TestsGrew, Change{f, b, a})
			}
		case a > b:
			d.Worsened = append(d.Worsened, Change{f, b, a})
		case a < b:
			d.Improved = append(d.Improved, Change{f, b, a})
		}
	}
	byMove := func(cs []Change) {
		sort.SliceStable(cs, func(i, j int) bool {
			mi, mj := abs(cs[i].After-cs[i].Before), abs(cs[j].After-cs[j].Before)
			if mi != mj {
				return mi > mj
			}
			return cs[i].File < cs[j].File
		})
	}
	byMove(d.Worsened)
	byMove(d.Improved)
	byMove(d.TestsGrew)
	return d
}

func abs(n int) int {
	if n < 0 {
		return -n
	}
	return n
}
