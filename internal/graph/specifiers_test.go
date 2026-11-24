package graph

import "testing"

// the report states how much of what the files import the graph could place, so
// the counts have to separate a package import, which is expected to leave the
// project, from a relative one that pointed inside it and found nothing.
func TestBuildCountsTheSpecifiersItKeptAndDropped(t *testing.T) {
	g, _ := edgeKinds(t, map[string]string{
		"a.ts": "import { b } from \"./b\";\nimport type { B } from \"./b\";\nimport React from \"react\";\nimport { gone } from \"./gone\";\nimport \"./styles.css\";\n",
		"b.ts": "export const b = 1;\nexport type B = 1;\n",
		"m.py": "from .n import x\nfrom .missing import y\nimport os\n",
		"n.py": "x = 1\n",
	})
	want := Specifiers{Kept: 3, Dropped: 5, DroppedRelative: 3}
	if g.Specifiers != want {
		t.Fatalf("specifiers = %+v, want %+v", g.Specifiers, want)
	}
}
