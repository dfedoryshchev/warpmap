package risk

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/dfedoryshchev/warpmap/internal/graph"
)

func p(s string) string { return filepath.FromSlash(s) }

// a.test.ts runs a.ts, and names t.ts only in an `import type`, which the
// compiler erases before the suite runs.
func typeOnlyGraph() graph.Graph {
	return graph.Graph{
		Files: []string{p("src/a.ts"), p("src/t.ts"), p("src/u.ts"), p("src/a.test.ts")},
		Edges: []graph.Edge{
			{From: p("src/a.test.ts"), To: p("src/a.ts")},
			{From: p("src/a.test.ts"), To: p("src/t.ts"), TypeOnly: true},
			{From: p("src/a.ts"), To: p("src/t.ts"), TypeOnly: true},
			{From: p("src/u.ts"), To: p("src/a.ts")},
		},
	}
}

func TestATypeOnlyImportFromATestDoesNotMakeAFileTested(t *testing.T) {
	var b strings.Builder
	Assess(".", typeOnlyGraph(), nil, []string{"src/a.ts", "src/t.ts", "src/u.ts"}, 100).Write(&b)
	want := "  src/a.ts: blast=2, shipped=1, tested\n" +
		"  src/t.ts: blast=3, shipped=2, types only, checked by the compiler\n" +
		"  src/u.ts: blast=0, shipped=0, UNTESTED\n"
	if got := b.String(); !strings.HasPrefix(got, want) {
		t.Fatalf("risk printed\n%swant it to start with\n%s", got, want)
	}
}

func TestGapsListTypeOnlyFilesUnderTheirOwnTag(t *testing.T) {
	var got []string
	for _, gap := range TestGaps(".", typeOnlyGraph(), nil) {
		got = append(got, filepath.ToSlash(gap.File))
	}
	if want := "src/t.ts src/u.ts"; strings.Join(got, " ") != want {
		t.Fatalf("gaps %q, want %q", strings.Join(got, " "), want)
	}
}

// y.ts and z.ts both have two dependents, but z.ts has two direct importers and
// y.ts one, reached through a chain.
func TestGapsBreakBlastTiesOnDirectImporters(t *testing.T) {
	g := graph.Graph{
		Files: []string{p("src/y.ts"), p("src/z.ts"), p("src/r.ts"), p("src/s.ts"), p("src/m.ts"), p("src/n.ts")},
		Edges: []graph.Edge{
			{From: p("src/r.ts"), To: p("src/y.ts")},
			{From: p("src/s.ts"), To: p("src/r.ts")},
			{From: p("src/m.ts"), To: p("src/z.ts")},
			{From: p("src/n.ts"), To: p("src/z.ts")},
		},
	}
	gaps := TestGaps(".", g, nil)
	if len(gaps) < 2 || filepath.ToSlash(gaps[0].File) != "src/z.ts" || filepath.ToSlash(gaps[1].File) != "src/y.ts" {
		t.Fatalf("gaps = %+v, want src/z.ts then src/y.ts at the top", gaps)
	}
}

// the test imports the barrel, so by imports alone the file behind it is
// untested; the coverage report says its lines ran.
func barrelGraph() graph.Graph {
	return graph.Graph{
		Files: []string{p("src/index.ts"), p("src/a.ts"), p("src/b.ts"), p("src/t.ts"), p("src/index.test.ts")},
		Edges: []graph.Edge{
			{From: p("src/index.test.ts"), To: p("src/index.ts")},
			{From: p("src/index.test.ts"), To: p("src/t.ts"), TypeOnly: true},
			{From: p("src/index.ts"), To: p("src/a.ts")},
			{From: p("src/index.ts"), To: p("src/b.ts")},
		},
	}
}

func TestExecutedLinesReplaceTheImportEdgeWhenThereIsAReport(t *testing.T) {
	ran := map[string]bool{p("src/index.ts"): true, p("src/a.ts"): true}
	var got []string
	for _, gap := range TestGaps(".", barrelGraph(), ran) {
		tag := ""
		if gap.TypesOnly {
			tag = "(types)"
		}
		got = append(got, filepath.ToSlash(gap.File)+tag)
	}
	if want := "src/b.ts src/t.ts(types)"; strings.Join(got, " ") != want {
		t.Fatalf("gaps %q, want %q", strings.Join(got, " "), want)
	}

	var b strings.Builder
	Assess(".", barrelGraph(), ran, []string{"src/a.ts", "src/b.ts"}, 100).Write(&b)
	want := "  src/a.ts: blast=2, shipped=1, tested\n  src/b.ts: blast=2, shipped=1, UNTESTED\n"
	if got := b.String(); !strings.HasPrefix(got, want) {
		t.Fatalf("risk printed\n%swant it to start with\n%s", got, want)
	}
}

// a test that imports a file whose lines the report says never ran did not run
// it: the report wins over the import.
func TestAnImportedFileNoLineOfWhichRanIsUntested(t *testing.T) {
	ran := map[string]bool{p("src/a.ts"): true}
	var got []string
	for _, gap := range TestGaps(".", barrelGraph(), ran) {
		got = append(got, filepath.ToSlash(gap.File))
	}
	if want := "src/b.ts src/index.ts src/t.ts"; strings.Join(got, " ") != want {
		t.Fatalf("gaps %q, want %q", strings.Join(got, " "), want)
	}
}
