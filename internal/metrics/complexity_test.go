package metrics

import (
	"path/filepath"
	"testing"
)

func complexityOf(t *testing.T, name, src string) Complexity {
	t.Helper()
	dir := t.TempDir()
	writeFiles(t, dir, map[string]string{name: src})
	c, err := FileComplexity(filepath.Join(dir, name))
	if err != nil {
		t.Fatal(err)
	}
	return c
}

func TestComplexityIgnoresComments(t *testing.T) {
	cases := []struct{ name, bare, commented string }{
		{
			"a.ts",
			"export function f(a: number) {\n  if (a > 1) return a\n  return 0\n}\n",
			"// if the value is empty && the cache is cold, return early\n" +
				"export function f(a: number) {\n" +
				"  /* for each case: while it is pending,\n" +
				"     catch the error || retry */\n" +
				"  if (a > 1) return a // if a is big\n" +
				"  return 0 /* ?? */\n" +
				"}\n",
		},
		{
			"a.tsx",
			"export const A = () => <p>{x && y}</p>\n",
			"export const A = () => <p>{/* if x && y */}{x && y}</p>\n// for later\n",
		},
		{
			"a.py",
			"def f(a):\n    if a: return a\n    return 0\n",
			"# if the value is empty and the cache is cold\n" +
				"def f(a):\n" +
				"    \"\"\"Return a.\n\n    if a is falsy, for any case, return 0.\n    \"\"\"\n" +
				"    if a: return a  # for a truthy a\n" +
				"    return 0\n",
		},
	}
	for _, c := range cases {
		bare := complexityOf(t, c.name, c.bare)
		commented := complexityOf(t, c.name, c.commented)
		if bare != commented {
			t.Errorf("%s: commented %+v, want the same as without comments %+v", c.name, commented, bare)
		}
	}
}

func TestComplexityKeepsCommentMarkersInsideStrings(t *testing.T) {
	cases := []struct {
		name, src string
		want      Complexity
	}{
		{"a.ts", "const u = \"http://x.io\"; if (u) go()\n", Complexity{LOC: 1, Branches: 1, Score: 4}},
		{"a.js", "const u = 'a/*b'; if (u) go() // */ if\n", Complexity{LOC: 1, Branches: 1, Score: 4}},
		{"a.ts", "const t = `//${x}`; if (t) go()\n", Complexity{LOC: 1, Branches: 1, Score: 4}},
		{"a.ts", "const r = /'\\/\\//; if (r) go()\n// for\n", Complexity{LOC: 1, Branches: 1, Score: 4}},
		{"a.tsx", "const A = <p>Don't</p>\n// if x\nif (a) go()\n", Complexity{LOC: 2, Branches: 1, Score: 5}},
		{"a.py", "u = \"#a\" if y else z\n", Complexity{LOC: 1, Branches: 1, Score: 4}},
		{"a.py", "s = '''\n# if\n'''\n", Complexity{LOC: 3, Branches: 1, Score: 6}},
	}
	for _, c := range cases {
		if got := complexityOf(t, c.name, c.src); got != c.want {
			t.Errorf("%s %q: got %+v, want %+v", c.name, c.src, got, c.want)
		}
	}
}
