package baseline

import "testing"

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

func TestCompareIgnoresAnEmptyNewFileAndARemovedFile(t *testing.T) {
	d := Compare(
		snap(map[string]int{"old.ts": 10, "gone.ts": 30}),
		snap(map[string]int{"old.ts": 10, "empty.ts": 0}),
	)
	if len(d.Worsened) != 0 || len(d.Improved) != 0 || d.RiskUp() {
		t.Fatalf("diff = %+v", d)
	}
}
