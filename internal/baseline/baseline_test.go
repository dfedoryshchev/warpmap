package baseline

import (
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

func snap(c map[string]int) Snapshot { return Snapshot{Complexity: c} }

func TestCompareDeltas(t *testing.T) {
	before := Snapshot{Edges: 10, Cycles: 1, Orphans: 2, GodModules: 0, Complexity: map[string]int{}}
	after := Snapshot{Edges: 7, Cycles: 3, Orphans: 5, GodModules: 1, Complexity: map[string]int{}}
	d := Compare(before, after)
	if d.Edges != -3 || d.Cycles != 2 || d.Orphans != 3 || d.GodModules != 1 {
		t.Fatalf("deltas = %+v", d)
	}
}

func TestCompareSplitsWorsenedAndImproved(t *testing.T) {
	d := Compare(
		snap(map[string]int{"a.ts": 10, "b.ts": 10, "c.ts": 10}),
		snap(map[string]int{"a.ts": 15, "b.ts": 4, "c.ts": 10}),
	)
	if len(d.Worsened) != 1 || d.Worsened[0] != (Change{"a.ts", 10, 15}) {
		t.Fatalf("worsened = %+v", d.Worsened)
	}
	if len(d.Improved) != 1 || d.Improved[0] != (Change{"b.ts", 10, 4}) {
		t.Fatalf("improved = %+v", d.Improved)
	}
}

// the Action posts what diff prints and re-posts when it changes, so files that
// moved by the same amount print in path order on every run, and a removed file
// pairs with the same added one each time.
func TestCompareIsTheSameOnEveryRun(t *testing.T) {
	before, after := map[string]int{"gone.ts": 5}, map[string]int{"m1.ts": 5, "m2.ts": 5}
	for i := 0; i < 20; i++ {
		before[fmt.Sprintf("up%02d.ts", i)] = 1
		after[fmt.Sprintf("up%02d.ts", i)] = 4
		before[fmt.Sprintf("down%02d.ts", i)] = 9
		after[fmt.Sprintf("down%02d.ts", i)] = 2
		before[fmt.Sprintf("t%02d.test.ts", i)] = 1
		after[fmt.Sprintf("t%02d.test.ts", i)] = 6
	}
	paths := func(cs []Change) []string {
		var out []string
		for _, c := range cs {
			out = append(out, c.File)
		}
		return out
	}
	for run := 0; run < 20; run++ {
		d := Compare(snap(before), snap(after))
		worsened := paths(d.Worsened)
		if worsened[0] != "m2.ts" || !slices.IsSorted(worsened[1:]) || len(worsened) != 21 {
			t.Fatalf("run %d: want m2.ts (the larger move) then the ties in path order, got %v", run, worsened)
		}
		if got := paths(d.Improved); !slices.IsSorted(got) || len(got) != 20 {
			t.Fatalf("run %d: improved ties not in path order: %v", run, got)
		}
		if got := paths(d.TestsGrew); !slices.IsSorted(got) || len(got) != 20 {
			t.Fatalf("run %d: test-file ties not in path order: %v", run, got)
		}
	}
}

func TestCompareOrdersByLargestMove(t *testing.T) {
	d := Compare(
		snap(map[string]int{"a.ts": 1, "b.ts": 1, "c.ts": 50, "d.ts": 50}),
		snap(map[string]int{"a.ts": 3, "b.ts": 9, "c.ts": 40, "d.ts": 10}),
	)
	if d.Worsened[0].File != "b.ts" || d.Worsened[1].File != "a.ts" {
		t.Fatalf("worsened order = %+v", d.Worsened)
	}
	if d.Improved[0].File != "d.ts" || d.Improved[1].File != "c.ts" {
		t.Fatalf("improved order = %+v", d.Improved)
	}
}

func TestRiskUpTerms(t *testing.T) {
	cases := []struct {
		name         string
		d            Diff
		risk, warned bool
	}{
		{"nothing moved", Diff{}, false, false},
		{"new cycle", Diff{Cycles: 1}, true, false},
		{"cycle removed", Diff{Cycles: -1}, false, false},
		{"new god module", Diff{GodModules: 1}, true, false},
		{"god module removed", Diff{GodModules: -1}, false, false},
		{"net complexity growth warns", Diff{Net: 1}, false, true},
		{"complexity balanced out", Diff{Net: 0}, false, false},
		{"net complexity drop", Diff{Net: -5}, false, false},
		{"new dead code", Diff{Orphans: 1}, true, false},
		{"dead code removed", Diff{Orphans: -2}, false, false},
		{"dead code removed but a cycle added", Diff{Orphans: -2, Cycles: 1}, true, false},
		{"a cycle outranks the warning", Diff{Cycles: 1, Net: 9}, true, false},
		{"edges alone do not trip it", Diff{Edges: 40}, false, false},
	}
	for _, c := range cases {
		if got := c.d.RiskUp(); got != c.risk {
			t.Errorf("%s: RiskUp = %v, want %v", c.name, got, c.risk)
		}
		if got := c.d.Warn(); got != c.warned {
			t.Errorf("%s: Warn = %v, want %v", c.name, got, c.warned)
		}
	}
}

func TestCompareWeighsComplexityByMagnitude(t *testing.T) {
	d := Compare(
		snap(map[string]int{"big.ts": 100, "a.ts": 10, "b.ts": 10, "gone.ts": 30}),
		snap(map[string]int{"big.ts": 1000, "a.ts": 9, "b.ts": 9, "new.ts": 5}),
	)
	if d.Net != 900-1-1+5-30 || !d.Warn() {
		t.Fatalf("net = %d, warn = %v; want %d and a warning", d.Net, d.Warn(), 900-1-1+5-30)
	}
}

func TestTermsNameWhatTrippedTheGate(t *testing.T) {
	d := Diff{Orphans: 2, GodModules: 1, Net: 1}
	var tripped []string
	for _, term := range d.Terms() {
		if term.Tripped {
			tripped = append(tripped, term.Name)
		}
	}
	want := []string{"god-modules", "complexity", "dead-code"}
	if len(tripped) != len(want) {
		t.Fatalf("tripped = %v, want %v", tripped, want)
	}
	for i := range want {
		if tripped[i] != want[i] {
			t.Fatalf("tripped = %v, want %v", tripped, want)
		}
	}
	if len(Diff{}.Terms()) != 5 {
		t.Fatalf("a quiet diff still lists every term: %+v", Diff{}.Terms())
	}
}

func TestCompareCountsANewFileAsWorsened(t *testing.T) {
	d := Compare(
		snap(map[string]int{"old.ts": 10}),
		snap(map[string]int{"old.ts": 10, "added.ts": 200}),
	)
	if len(d.Worsened) != 1 || d.Worsened[0] != (Change{"added.ts", 0, 200}) {
		t.Fatalf("worsened = %+v", d.Worsened)
	}
	if d.Net != 200 || !d.Warn() {
		t.Fatalf("a 200-point file added with nothing else moved: net %d, warn %v", d.Net, d.Warn())
	}
}

func TestCompareFunctionsPairsRepeatedNamesInOrder(t *testing.T) {
	cb := func(ccn, length int) Function {
		return Function{File: "a.ts", Name: "(anonymous)", CCN: ccn, Length: length}
	}
	before := []Function{cb(1, 3), cb(2, 5)}
	after := []Function{cb(1, 3), cb(2, 9), cb(4, 4)}
	got := CompareFunctions(before, after)
	if len(got) != 2 {
		t.Fatalf("want the second callback grown and a third added, got %+v", got)
	}
	if got[0].Before != nil || got[0].After != cb(4, 4) {
		t.Fatalf("first = %+v, want the added one (highest CCN)", got[0])
	}
	if got[1].Before == nil || *got[1].Before != cb(2, 5) || got[1].After != cb(2, 9) {
		t.Fatalf("second = %+v, want the second callback 5 -> 9 lines", got[1])
	}
}

func TestASnapshotWithoutFunctionsLoadsAsUnmeasured(t *testing.T) {
	dir := t.TempDir()
	if err := os.MkdirAll(filepath.Dir(Path(dir)), 0o755); err != nil {
		t.Fatal(err)
	}
	old := `{"files":1,"edges":0,"cycles":0,"orphans":0,"godModules":0,"complexity":{"a.ts":3}}`
	if err := os.WriteFile(Path(dir), []byte(old), 0o644); err != nil {
		t.Fatal(err)
	}
	s, err := Load(dir)
	if err != nil || s.Functions != nil || s.Complexity["a.ts"] != 3 {
		t.Fatalf("Load = %+v, %v; want the old fields and nil Functions", s, err)
	}
	if err := Save(dir, Snapshot{Functions: []Function{}}); err != nil {
		t.Fatal(err)
	}
	if s, err = Load(dir); err != nil || s.Functions == nil {
		t.Fatalf("a measured snapshot with no functions loaded as unmeasured: %+v, %v", s, err)
	}
}

func TestCompareKeepsTestFilesOutOfTheComplexityTerm(t *testing.T) {
	d := Compare(
		snap(map[string]int{"src/a.ts": 10, "src/a.test.ts": 10, "tests/setup.ts": 5, "src/b.spec.ts": 40}),
		snap(map[string]int{"src/a.ts": 10, "src/a.test.ts": 90, "tests/setup.ts": 9, "src/b.spec.ts": 20, "src/c.test.ts": 30}),
	)
	if len(d.Worsened) != 0 || len(d.Improved) != 0 {
		t.Fatalf("worsened = %+v, improved = %+v; want test files in neither", d.Worsened, d.Improved)
	}
	want := []Change{{"src/a.test.ts", 10, 90}, {"src/c.test.ts", 0, 30}, {"tests/setup.ts", 5, 9}}
	if len(d.TestsGrew) != len(want) {
		t.Fatalf("tests grew = %+v, want %+v", d.TestsGrew, want)
	}
	for i := range want {
		if d.TestsGrew[i] != want[i] {
			t.Fatalf("tests grew = %+v, want %+v", d.TestsGrew, want)
		}
	}
	if d.RiskUp() {
		t.Fatal("growing only test files tripped the gate")
	}
}

func TestCompareStillTripsOnASourceFileNextToGrowingTests(t *testing.T) {
	d := Compare(
		snap(map[string]int{"src/a.ts": 10, "src/a.test.ts": 10}),
		snap(map[string]int{"src/a.ts": 25, "src/a.test.ts": 90}),
	)
	if len(d.Worsened) != 1 || d.Worsened[0] != (Change{"src/a.ts", 10, 25}) {
		t.Fatalf("worsened = %+v", d.Worsened)
	}
	if d.Net != 15 || !d.Warn() {
		t.Fatalf("a source file grew by 15: net %d, warn %v", d.Net, d.Warn())
	}
}

func TestCompareIgnoresAnEmptyNewFileAndARemovedFile(t *testing.T) {
	d := Compare(
		snap(map[string]int{"old.ts": 10, "gone.ts": 30}),
		snap(map[string]int{"old.ts": 10, "empty.ts": 0}),
	)
	if len(d.Worsened) != 0 || len(d.Improved) != 0 || d.RiskUp() {
		t.Fatalf("diff = %+v", d)
	}
}

func TestCompareTakesAFileThatMovedAsMoved(t *testing.T) {
	d := Compare(
		snap(map[string]int{"src/a.ts": 10, "src/d.ts": 12, "src/d.test.ts": 7}),
		snap(map[string]int{"src/a.ts": 10, "src/lib/d.ts": 12, "src/lib/d.test.ts": 7}),
	)
	if len(d.Worsened) != 0 || len(d.TestsGrew) != 0 || d.RiskUp() {
		t.Fatalf("moving two files unchanged read as adding them: %+v", d)
	}
}

// deps builds a snapshot over the given files with the given value and
// type-only imports, each written "from>to".
func deps(files []string, imports, typeImports []string) Snapshot {
	s := Snapshot{Complexity: map[string]int{}, Imports: map[string][]string{}, TypeImports: map[string][]string{}}
	for _, f := range files {
		s.Complexity[f] = 1
	}
	add := func(m map[string][]string, pairs []string) {
		for _, p := range pairs {
			from, to, _ := strings.Cut(p, ">")
			m[from] = append(m[from], to)
		}
	}
	add(s.Imports, imports)
	add(s.TypeImports, typeImports)
	return s
}

func tripped(d Diff, name string) bool {
	for _, term := range d.Terms() {
		if term.Name == name {
			return term.Tripped
		}
	}
	return false
}

var depFiles = []string{"ui/form.ts", "db/conn.ts", "db/conn.test.ts", "app/main.ts", "tests/db.test.ts"}

func TestCompareNamesANewDependencyBetweenModules(t *testing.T) {
	before := deps(depFiles, []string{"app/main.ts>db/conn.ts", "db/conn.test.ts>db/conn.ts"}, nil)
	after := deps(depFiles, []string{
		"app/main.ts>db/conn.ts", "db/conn.test.ts>db/conn.ts",
		"ui/form.ts>db/conn.ts", "tests/db.test.ts>db/conn.ts",
	}, nil)
	d := Compare(before, after)
	want := []ModuleDep{{From: "ui", To: "db", Imports: 1, FromFile: "ui/form.ts", ToFile: "db/conn.ts", Dependents: 2}}
	if !slices.Equal(d.NewDeps, want) {
		t.Fatalf("new deps = %+v, want %+v (a test importer neither counts as a dependent nor opens a pair)", d.NewDeps, want)
	}
	if !tripped(d, "new-dependencies") || d.RiskUp() || !d.Warn() {
		t.Fatalf("a new value dependency between existing modules should warn: risk %v, warn %v", d.RiskUp(), d.Warn())
	}
}

func TestCompareListsATypeOnlyModuleDependencyWithoutTrippingTheTerm(t *testing.T) {
	before := deps(depFiles, []string{"app/main.ts>db/conn.ts"}, nil)
	after := deps(depFiles, []string{"app/main.ts>db/conn.ts"}, []string{"ui/form.ts>db/conn.ts"})
	d := Compare(before, after)
	if len(d.NewDeps) != 1 || !d.NewDeps[0].TypeOnly || d.NewDeps[0].From != "ui" {
		t.Fatalf("new deps = %+v, want ui -> db marked type-only", d.NewDeps)
	}
	if tripped(d, "new-dependencies") || d.Warn() {
		t.Fatal("a type-only module dependency tripped the term")
	}
}

func TestCompareListsADependencyOnANewModuleWithoutTrippingTheTerm(t *testing.T) {
	before := deps(depFiles, []string{"app/main.ts>db/conn.ts"}, nil)
	after := deps(append(slices.Clone(depFiles), "billing/charge.ts"),
		[]string{"app/main.ts>db/conn.ts", "app/main.ts>billing/charge.ts", "billing/charge.ts>db/conn.ts"}, nil)
	d := Compare(before, after)
	if len(d.NewDeps) != 2 || !d.NewDeps[0].NewModule || !d.NewDeps[1].NewModule {
		t.Fatalf("new deps = %+v, want both pairs touching billing marked as a new module", d.NewDeps)
	}
	if tripped(d, "new-dependencies") {
		t.Fatal("wiring in a module the change created tripped the term")
	}
}

func TestCompareRanksNewDependenciesByTheTargetsDependents(t *testing.T) {
	files := []string{"ui/a.ts", "util/fmt.ts", "db/conn.ts", "app/x.ts", "app/y.ts", "api/z.ts"}
	before := deps(files, []string{"app/x.ts>db/conn.ts", "app/y.ts>db/conn.ts", "api/z.ts>app/x.ts", "api/z.ts>util/fmt.ts"}, nil)
	after := deps(files, []string{
		"app/x.ts>db/conn.ts", "app/y.ts>db/conn.ts", "api/z.ts>app/x.ts", "api/z.ts>util/fmt.ts",
		"ui/a.ts>util/fmt.ts", "ui/a.ts>db/conn.ts",
	}, nil)
	d := Compare(before, after)
	if len(d.NewDeps) != 2 || d.NewDeps[0].To != "db" || d.NewDeps[1].To != "util" {
		t.Fatalf("new deps = %+v, want ui -> db first", d.NewDeps)
	}
	if d.NewDeps[0].Dependents != 4 || d.NewDeps[1].Dependents != 2 {
		t.Fatalf("dependents = %d and %d, want 4 (app/x, app/y, api/z, ui/a) and 2 (api/z, ui/a)",
			d.NewDeps[0].Dependents, d.NewDeps[1].Dependents)
	}
}

func TestCompareAgainstABaselineWithNoImportListListsNothing(t *testing.T) {
	before := snap(map[string]int{"ui/form.ts": 1, "db/conn.ts": 1})
	after := deps([]string{"ui/form.ts", "db/conn.ts"}, []string{"ui/form.ts>db/conn.ts"}, nil)
	d := Compare(before, after)
	if !d.DepsUnknown || d.NewDeps != nil || tripped(d, "new-dependencies") {
		t.Fatalf("diff = %+v, want the import list reported unknown and nothing listed", d)
	}
}

func TestCaptureKeepsEveryImportSortedWithTheTypeOnlyOnesApart(t *testing.T) {
	dir := t.TempDir()
	write := func(name, src string) string {
		p := filepath.Join(dir, filepath.FromSlash(name))
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte(src), 0o644); err != nil {
			t.Fatal(err)
		}
		return p
	}
	files := []string{
		write("db/conn.ts", "export const conn = 1;\nexport type Conn = number;\n"),
		write("db/pool.ts", "export const pool = 1;\n"),
		write("ui/form.ts", "import { pool } from \"../db/pool\";\nimport { conn } from \"../db/conn\";\nimport type { Conn } from \"../db/conn\";\nexport const f = conn + pool;\n"),
		write("ui/view.ts", "import type { Conn } from \"../db/conn\";\nexport const v = 1;\n"),
	}
	s := Capture(dir, files)
	if got := s.Imports["ui/form.ts"]; !slices.Equal(got, []string{"db/conn.ts", "db/pool.ts"}) {
		t.Fatalf("imports of ui/form.ts = %v, want both, sorted", got)
	}
	if got := s.TypeImports["ui/view.ts"]; !slices.Equal(got, []string{"db/conn.ts"}) {
		t.Fatalf("type imports of ui/view.ts = %v", got)
	}
	if _, ok := s.Imports["ui/view.ts"]; ok {
		t.Fatal("a type-only import was stored as a value import too")
	}
	if err := Save(dir, s); err != nil {
		t.Fatal(err)
	}
	loaded, err := Load(dir)
	if err != nil || loaded.Imports == nil || !slices.Equal(loaded.Imports["ui/form.ts"], s.Imports["ui/form.ts"]) {
		t.Fatalf("import list did not survive a save: %+v, %v", loaded.Imports, err)
	}
	if empty := Capture(dir, files[:2]); empty.Imports == nil {
		t.Fatal("a project with no imports captured an unknown import list")
	}
}

func TestCompareStillCountsAFileThatMovedAndGrew(t *testing.T) {
	d := Compare(
		snap(map[string]int{"src/d.ts": 12}),
		snap(map[string]int{"src/lib/d.ts": 20}),
	)
	if len(d.Worsened) != 1 || d.Worsened[0] != (Change{"src/lib/d.ts", 0, 20}) || d.Net != 8 || !d.Warn() {
		t.Fatalf("diff = %+v", d)
	}
}
