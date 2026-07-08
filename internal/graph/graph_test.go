package graph

import "testing"

func sample() Graph {
	// b imports a, c imports b  (an acyclic chain)
	return Graph{
		Files: []string{"a.ts", "b.ts", "c.ts"},
		Edges: []Edge{{From: "b.ts", To: "a.ts"}, {From: "c.ts", To: "b.ts"}},
	}
}

func TestOrphans(t *testing.T) {
	got := Orphans(sample())
	if len(got) != 1 || got[0] != "c.ts" {
		t.Fatalf("want [c.ts] (nothing imports it), got %v", got)
	}
}

func TestCyclesFindsCycle(t *testing.T) {
	cyclic := Graph{
		Files: []string{"x.ts", "y.ts"},
		Edges: []Edge{{From: "x.ts", To: "y.ts"}, {From: "y.ts", To: "x.ts"}},
	}
	if len(Cycles(cyclic)) == 0 {
		t.Fatal("expected a cycle between x and y")
	}
	if got := Cycles(sample()); len(got) != 0 {
		t.Fatalf("acyclic graph should have no cycles, got %v", got)
	}
}

func TestCyclesIgnoresSelfImport(t *testing.T) {
	self := Graph{Files: []string{"z.ts"}, Edges: []Edge{{From: "z.ts", To: "z.ts"}}}
	if got := Cycles(self); len(got) != 0 {
		t.Fatalf("a self-import is not a real cycle, got %v", got)
	}
}

func TestGodModules(t *testing.T) {
	if len(GodModules(sample())) == 0 {
		t.Fatal("expected fan-in/out to rank some modules")
	}
}
