package baseline

import (
	"os"
	"path/filepath"
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
	one := []Change{{"x.ts", 1, 2}}
	cases := []struct {
		name string
		d    Diff
		want bool
	}{
		{"nothing moved", Diff{}, false},
		{"new cycle", Diff{Cycles: 1}, true},
		{"cycle removed", Diff{Cycles: -1}, false},
		{"new god module", Diff{GodModules: 1}, true},
		{"god module removed", Diff{GodModules: -1}, false},
		{"more worsened than improved", Diff{Worsened: one}, true},
		{"worsened balanced by improved", Diff{Worsened: one, Improved: one}, false},
		{"more improved than worsened", Diff{Improved: one}, false},
		{"orphans alone do not trip it", Diff{Orphans: 4}, false},
		{"edges alone do not trip it", Diff{Edges: 40}, false},
	}
	for _, c := range cases {
		if got := c.d.RiskUp(); got != c.want {
			t.Errorf("%s: RiskUp = %v, want %v", c.name, got, c.want)
		}
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
	if !d.RiskUp() {
		t.Fatal("a 200-point file added with nothing else moved passed the gate")
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

func TestCompareIgnoresAnEmptyNewFileAndARemovedFile(t *testing.T) {
	d := Compare(
		snap(map[string]int{"old.ts": 10, "gone.ts": 30}),
		snap(map[string]int{"old.ts": 10, "empty.ts": 0}),
	)
	if len(d.Worsened) != 0 || len(d.Improved) != 0 || d.RiskUp() {
		t.Fatalf("diff = %+v", d)
	}
}
