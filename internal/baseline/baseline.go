package baseline

import (
	"encoding/json"
	"fmt"
	"maps"
	"os"
	"path"
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
	// Imports maps each importing file to the files it imports, project-relative
	// and sorted; TypeImports holds the imports that name only types. Both are nil
	// in a snapshot written before they existed.
	Imports     map[string][]string `json:"imports"`
	TypeImports map[string][]string `json:"typeImports"`
}

// ModuleDep is a dependency between two modules (directories) that had no import
// between them at the baseline.
type ModuleDep struct {
	From, To string
	// Imports counts the file imports behind the pair; FromFile and ToFile name
	// the first of them.
	Imports          int
	FromFile, ToFile string
	TypeOnly         bool // every import behind the pair names only types
	NewModule        bool // one end did not exist at the baseline
	// Dependents counts the shipped files outside To that depend on it, directly
	// or through other files.
	Dependents int
}

// Counts reports whether the pair counts towards the verdict: a type-only pair,
// and a pair that wires in a module the change created, are listed but do not.
func (m ModuleDep) Counts() bool { return !m.TypeOnly && !m.NewModule }

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
		Files:       len(g.Files),
		Edges:       len(g.Edges),
		Cycles:      len(graph.Cycles(g)),
		Orphans:     len(graph.Orphans(dir, g)),
		GodModules:  godCount(g),
		Complexity:  map[string]int{},
		Imports:     map[string][]string{},
		TypeImports: map[string][]string{},
	}
	rel := func(f string) string {
		r, err := filepath.Rel(dir, f)
		if err != nil {
			r = f
		}
		return filepath.ToSlash(r)
	}
	for _, f := range files {
		c, err := metrics.FileComplexity(f)
		if err != nil {
			continue
		}
		s.Complexity[rel(f)] = c.Score
	}
	for _, e := range g.Edges {
		m := s.Imports
		if e.TypeOnly {
			m = s.TypeImports
		}
		from := rel(e.From)
		m[from] = append(m[from], rel(e.To))
	}
	for _, m := range []map[string][]string{s.Imports, s.TypeImports} {
		for _, to := range m {
			slices.Sort(to)
		}
	}
	return s
}

func module(file string) string { return path.Dir(file) }

// shippedDependents counts, for each module, the shipped files outside it that
// depend on one of its files through any chain of imports.
func shippedDependents(s Snapshot, wanted map[string]bool) map[string]int {
	rev := map[string][]string{}
	members := map[string][]string{}
	seenFile := map[string]bool{}
	note := func(f string) {
		if !seenFile[f] {
			seenFile[f] = true
			if m := module(f); wanted[m] {
				members[m] = append(members[m], f)
			}
		}
	}
	for f := range s.Complexity {
		note(f)
	}
	for _, m := range []map[string][]string{s.Imports, s.TypeImports} {
		for from, tos := range m {
			note(from)
			for _, to := range tos {
				note(to)
				rev[to] = append(rev[to], from)
			}
		}
	}
	out := map[string]int{}
	for mod := range wanted {
		seen := map[string]bool{}
		frontier := members[mod]
		for _, f := range frontier {
			seen[f] = true
		}
		for len(frontier) > 0 {
			var next []string
			for _, f := range frontier {
				for _, importer := range rev[f] {
					if !seen[importer] {
						seen[importer] = true
						next = append(next, importer)
					}
				}
			}
			frontier = next
		}
		for f := range seen {
			if module(f) != mod && !coverage.IsTest(f) {
				out[mod]++
			}
		}
	}
	return out
}

// newModuleDeps lists the module pairs after has an import between and before
// has none. Imports from or into a test file are left out: a test reaching into
// another directory is not the code taking on a dependency.
func newModuleDeps(before, after Snapshot) []ModuleDep {
	type pair struct{ from, to string }
	pairs := func(s Snapshot, each func(p pair, from, to string, typeOnly bool)) {
		for _, kind := range []struct {
			m        map[string][]string
			typeOnly bool
		}{{s.Imports, false}, {s.TypeImports, true}} {
			for _, from := range slices.Sorted(maps.Keys(kind.m)) {
				for _, to := range kind.m[from] {
					p := pair{module(from), module(to)}
					if p.from == p.to || coverage.IsTest(from) || coverage.IsTest(to) {
						continue
					}
					each(p, from, to, kind.typeOnly)
				}
			}
		}
	}
	had := map[pair]bool{}
	pairs(before, func(p pair, _, _ string, _ bool) { had[p] = true })
	existed := map[string]bool{}
	for f := range before.Complexity {
		existed[module(f)] = true
	}
	for _, m := range []map[string][]string{before.Imports, before.TypeImports} {
		for from, tos := range m {
			existed[module(from)] = true
			for _, to := range tos {
				existed[module(to)] = true
			}
		}
	}
	found := map[pair]*ModuleDep{}
	pairs(after, func(p pair, from, to string, typeOnly bool) {
		if had[p] {
			return
		}
		d := found[p]
		if d == nil {
			d = &ModuleDep{From: p.from, To: p.to, FromFile: from, ToFile: to, TypeOnly: true,
				NewModule: !existed[p.from] || !existed[p.to]}
			found[p] = d
		}
		d.Imports++
		if !typeOnly {
			if d.TypeOnly {
				d.FromFile, d.ToFile = from, to
			}
			d.TypeOnly = false
		}
	})
	if len(found) == 0 {
		return nil
	}
	targets := map[string]bool{}
	for p := range found {
		targets[p.to] = true
	}
	dependents := shippedDependents(after, targets)
	out := make([]ModuleDep, 0, len(found))
	for _, d := range found {
		d.Dependents = dependents[d.To]
		out = append(out, *d)
	}
	sort.Slice(out, func(i, j int) bool {
		a, b := out[i], out[j]
		if a.Dependents != b.Dependents {
			return a.Dependents > b.Dependents
		}
		if a.From != b.From {
			return a.From < b.From
		}
		return a.To < b.To
	})
	return out
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
	// Net is the complexity the shipped files gained, less what they lost: the
	// worsened deltas, minus the improved ones and the scores of removed files.
	Net int
	// NewDeps lists module pairs with no import at the baseline and some now,
	// the most depended-on target first. DepsUnknown is set when the baseline
	// holds no import list to compare against.
	NewDeps     []ModuleDep
	DepsUnknown bool
}

// Term is one named condition of the verdict. A Warn term only ever warns: a
// feature adds code, so complexity growing is the normal shape of a change.
type Term struct {
	Name    string
	Tripped bool
	Warn    bool
}

// Terms lists every condition the verdict is made of, tripped or not. Each term
// carries its own exception: complexity leaves test files out, dead code is
// counted on net growth, entry points (see graph.Orphans) never count as dead,
// and a new module dependency counts only when ModuleDep.Counts says so.
func (d Diff) Terms() []Term {
	return []Term{
		{"cycles", d.Cycles > 0, false},
		{"god-modules", d.GodModules > 0, false},
		{"complexity", d.Net > 0, true},
		{"dead-code", d.Orphans > 0, false},
		{"new-dependencies", slices.ContainsFunc(d.NewDeps, ModuleDep.Counts), true},
	}
}

// RiskUp reports whether the change made the codebase harder to work on safely:
// whether any term that is not a warning tripped.
func (d Diff) RiskUp() bool {
	for _, t := range d.Terms() {
		if t.Tripped && !t.Warn {
			return true
		}
	}
	return false
}

// Warn reports whether only warning terms tripped.
func (d Diff) Warn() bool {
	if d.RiskUp() {
		return false
	}
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
	if before.Imports == nil {
		d.DepsUnknown = true
	} else {
		d.NewDeps = newModuleDeps(before, after)
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
	for k, n := range removed {
		if !k.test {
			d.Net -= k.score * n
		}
	}
	for _, c := range d.Worsened {
		d.Net += c.After - c.Before
	}
	for _, c := range d.Improved {
		d.Net -= c.Before - c.After
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
