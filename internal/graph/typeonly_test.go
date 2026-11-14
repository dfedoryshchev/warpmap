package graph

import (
	"path/filepath"
	"strings"
	"testing"
)

// edgeKinds builds the graph of a project written to a temp dir and names each
// edge "from->to" with whether it is type-only.
func edgeKinds(t *testing.T, files map[string]string) (Graph, map[string]bool) {
	t.Helper()
	dir := t.TempDir()
	var paths []string
	for name, body := range files {
		p := filepath.Join(dir, name)
		writeFile(t, p, body)
		paths = append(paths, p)
	}
	g := Build(paths)
	kinds := map[string]bool{}
	for _, e := range g.Edges {
		kinds[filepath.Base(e.From)+"->"+filepath.Base(e.To)] = e.TypeOnly
	}
	return g, kinds
}

func TestImportsThatOnlyNameTypesAreMarkedTypeOnly(t *testing.T) {
	_, kinds := edgeKinds(t, map[string]string{
		"t1.ts":     "import type { A } from \"./target\";\n",
		"t2.ts":     "export type { A } from \"./target\";\n",
		"t3.ts":     "import { type A, type B } from \"./target\";\n",
		"t4.ts":     "import type * as T from \"./target\";\n",
		"v1.ts":     "import { type A, b } from \"./target\";\n",
		"v2.ts":     "import { b } from \"./target\";\n",
		"v3.ts":     "import type from \"./target\";\n",
		"v4.ts":     "import d, { type A } from \"./target\";\n",
		"v5.ts":     "import type { A } from \"./target\";\nimport { b } from \"./target\";\n",
		"v6.ts":     "export * from \"./target\";\n",
		"v7.ts":     "import { type as T } from \"./target\";\n",
		"target.ts": "export type A = 1;\nexport type B = 2;\nexport const b = 1;\nexport default 1;\n",
	})
	for name, want := range map[string]bool{
		"t1": true, "t2": true, "t3": true, "t4": true,
		"v1": false, "v2": false, "v3": false, "v4": false, "v5": false, "v6": false, "v7": false,
	} {
		got, ok := kinds[name+".ts->target.ts"]
		if !ok {
			t.Errorf("%s: no edge to target.ts: %v", name, kinds)
			continue
		}
		if got != want {
			t.Errorf("%s: type-only = %v, want %v", name, got, want)
		}
	}
}

// a loop closed only by `import type` vanishes at compile time, so it is no
// tangle; the edge stays in the graph, and in the dot output, marked.
func TestATypeOnlyEdgeClosesNoCycle(t *testing.T) {
	g, _ := edgeKinds(t, map[string]string{
		"a.ts": "import { b } from \"./b\";\nexport const a = b;\n",
		"b.ts": "import type { A } from \"./a\";\nexport const b = 1;\nexport type B = A;\n",
		"c.ts": "import { d } from \"./d\";\nexport const c = d;\n",
		"d.ts": "import { c } from \"./c\";\nexport const d = c;\n",
	})
	if len(g.Edges) != 4 {
		t.Fatalf("want all 4 edges kept, got %v", g.Edges)
	}
	cycles := Cycles(g)
	if len(cycles) != 1 || filepath.Base(cycles[0].Files[0]) != "c.ts" {
		t.Fatalf("want only the c/d value cycle, got %+v", cycles)
	}
	dot := ToDot(g)
	var dashed int
	for _, line := range strings.Split(dot, "\n") {
		if strings.Contains(line, "style=dashed") {
			dashed++
			if !strings.Contains(line, "b.ts") {
				t.Fatalf("a value edge is drawn dashed: %s", line)
			}
		}
	}
	if dashed != 1 {
		t.Fatalf("want the one type-only edge dashed in\n%s", dot)
	}
}
