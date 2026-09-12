package dashboard

import (
	"fmt"
	"math"
	"regexp"
	"strconv"
	"strings"
	"testing"
)

func items(paths ...string) []Item {
	out := make([]Item, 0, len(paths))
	for i, p := range paths {
		out = append(out, Item{Path: p, Complexity: 10 * (len(paths) - i), Churn: i + 1})
	}
	return out
}

func TestTreeGroupsByDirectory(t *testing.T) {
	root := Tree([]Item{
		{Path: "src/api/client.ts", Complexity: 15},
		{Path: "src/store/session.ts", Complexity: 25},
		{Path: "index.ts", Complexity: 4},
	})
	if len(root.Children) != 2 {
		t.Fatalf("root children = %d, want 2 (src, index.ts)", len(root.Children))
	}
	// biggest first: src totals 40, index.ts is 4.
	src := root.Children[0]
	if src.Name != "src" || src.Size != 40 {
		t.Fatalf("first child = %q size %g, want src size 40", src.Name, src.Size)
	}
	if root.Children[1].Name != "index.ts" {
		t.Fatalf("second child = %q, want index.ts", root.Children[1].Name)
	}
	if len(src.Children) != 2 {
		t.Fatalf("src children = %d, want 2", len(src.Children))
	}
	if got := src.Children[0].Path; got != "src/store" {
		t.Fatalf("src's biggest child = %q, want src/store", got)
	}
}

// an empty file has complexity 0. it has to stay on the map.
func TestEmptyFileStillGetsABox(t *testing.T) {
	root := Tree([]Item{{Path: "a.ts", Complexity: 40}, {Path: "empty.ts", Complexity: 0}})
	Layout(root, 100, 100)
	for _, c := range root.Children {
		if c.Name != "empty.ts" {
			continue
		}
		if c.Size != 1 {
			t.Fatalf("empty file size = %g, want 1", c.Size)
		}
		if c.Rect.W <= 0 || c.Rect.H <= 0 {
			t.Fatalf("empty file got no box: %+v", c.Rect)
		}
		return
	}
	t.Fatal("empty.ts is not on the map at all")
}

// a flat project is the case where the areas must be exactly right: no
// directory frame eats into the canvas, so every box is its share of it.
func TestFlatLayoutGivesEachFileItsShareOfTheCanvas(t *testing.T) {
	root := Tree(items("a.ts", "b.ts", "c.ts", "d.ts", "e.ts"))
	Layout(root, 800, 500)
	total := 0.0
	for _, c := range root.Children {
		total += c.Size
	}
	for _, c := range root.Children {
		want := c.Size / total * 800 * 500
		got := c.Rect.W * c.Rect.H
		if math.Abs(got-want) > 0.5 {
			t.Fatalf("%s area = %.2f, want %.2f", c.Name, got, want)
		}
	}
}

func TestLayoutBoxesStayInsideTheirParentAndDoNotOverlap(t *testing.T) {
	root := Tree(items(
		"src/api/client.ts", "src/api/index.ts", "src/store/session.ts",
		"src/store/index.ts", "src/ui/Widget.tsx", "src/ui/deep/nested/Leaf.tsx",
		"src/util/format.ts", "tests/session.test.ts", "index.ts",
	))
	Layout(root, canvasW, canvasH)

	var leaves []*Node
	var walk func(n *Node)
	walk = func(n *Node) {
		if n.Leaf() {
			leaves = append(leaves, n)
			return
		}
		for _, c := range n.Children {
			if !inside(c.Rect, n.Rect) {
				t.Fatalf("%s (%+v) escapes its parent %s (%+v)", c.Path, c.Rect, n.Path, n.Rect)
			}
			walk(c)
		}
	}
	walk(root)

	if len(leaves) != 9 {
		t.Fatalf("laid out %d leaves, want 9", len(leaves))
	}
	for i := range leaves {
		for j := i + 1; j < len(leaves); j++ {
			if a := overlap(leaves[i].Rect, leaves[j].Rect); a > 0.01 {
				t.Fatalf("%s and %s overlap by %.3f", leaves[i].Path, leaves[j].Path, a)
			}
		}
	}
}

func inside(inner, outer Rect) bool {
	const eps = 0.001
	return inner.X >= outer.X-eps && inner.Y >= outer.Y-eps &&
		inner.X+inner.W <= outer.X+outer.W+eps &&
		inner.Y+inner.H <= outer.Y+outer.H+eps
}

func overlap(a, b Rect) float64 {
	w := math.Min(a.X+a.W, b.X+b.W) - math.Max(a.X, b.X)
	h := math.Min(a.Y+a.H, b.Y+b.H) - math.Max(a.Y, b.Y)
	if w <= 0 || h <= 0 {
		return 0
	}
	return w * h
}

// deepProject is a feature-sliced tree six directories deep, the shape a
// front-end grows into and the one a treemap is asked to make legible. The
// complexities are the areas: they decide the boxes, so they are fixed.
var deepProject = []Item{
	{Path: "src/app/providers/router/routes/private/Guard.tsx", Complexity: 35},
	{Path: "src/app/providers/router/routes/private/Routes.tsx", Complexity: 200},
	{Path: "src/app/providers/router/routes/public/Routes.tsx", Complexity: 21},
	{Path: "src/app/providers/store/slices/session/slice.ts", Complexity: 8},
	{Path: "src/app/providers/store/slices/session/selectors.ts", Complexity: 88},
	{Path: "src/app/providers/store/slices/session/thunks.ts", Complexity: 155},
	{Path: "src/app/providers/store/slices/catalog/slice.ts", Complexity: 53},
	{Path: "src/app/providers/store/slices/catalog/selectors.ts", Complexity: 91},
	{Path: "src/features/checkout/components/forms/fields/AddressField.tsx", Complexity: 211},
	{Path: "src/features/checkout/components/forms/fields/CardNumberField.tsx", Complexity: 11},
	{Path: "src/features/checkout/components/forms/fields/ExpiryField.tsx", Complexity: 176},
	{Path: "src/features/checkout/components/forms/validation/rules.ts", Complexity: 10},
	{Path: "src/features/checkout/components/forms/validation/messages.ts", Complexity: 51},
	{Path: "src/features/checkout/model/types.ts", Complexity: 29},
	{Path: "src/features/checkout/model/reducer.ts", Complexity: 174},
	{Path: "src/features/catalog/components/grid/cells/PriceCell.tsx", Complexity: 194},
	{Path: "src/features/catalog/components/grid/cells/TitleCell.tsx", Complexity: 30},
	{Path: "src/features/catalog/model/types.ts", Complexity: 109},
	{Path: "src/entities/user/api/client.ts", Complexity: 9},
	{Path: "src/entities/user/ui/Avatar.tsx", Complexity: 213},
	{Path: "src/shared/lib/format/currency/format.ts", Complexity: 217},
	{Path: "src/shared/lib/format/date/format.ts", Complexity: 125},
	{Path: "src/shared/ui/button/Button.tsx", Complexity: 61},
	{Path: "tests/e2e/checkout/flow.spec.ts", Complexity: 181},
	{Path: "index.ts", Complexity: 219},
}

type drawnLabel struct {
	text string
	ink  Rect
}

var labelRe = regexp.MustCompile(
	`<text class="(?:dlabel|flabel)" x="([-0-9.]+)" y="([-0-9.]+)"[^>]*>([^<]*)</text>`)

// drawnLabels reads the labels back off the finished page, because that is the
// only place their final coordinates exist. The font is 10px monospace, so one
// rune is charW wide and the ink of a single line sits between the ascender and
// the descender of the baseline the label is anchored to.
func drawnLabels(t *testing.T, page string) []drawnLabel {
	t.Helper()
	const ascent, descent = 7.5, 2.5
	var out []drawnLabel
	for _, m := range labelRe.FindAllStringSubmatch(page, -1) {
		x, err := strconv.ParseFloat(m[1], 64)
		if err != nil {
			t.Fatal(err)
		}
		y, err := strconv.ParseFloat(m[2], 64)
		if err != nil {
			t.Fatal(err)
		}
		out = append(out, drawnLabel{
			text: m[3],
			ink:  Rect{X: x, Y: y - ascent, W: charW * float64(len([]rune(m[3]))), H: ascent + descent},
		})
	}
	return out
}

// a label drawn on top of another label is the one failure that makes the map
// less readable than the table under it, and deep nesting is where it happens:
// a directory box can be tall enough to draw its own name and still too short
// to keep a strip to draw it in.
func TestDeepNestingDrawsNoLabelOnTopOfAnother(t *testing.T) {
	page := Render(Page{Project: "p", Files: len(deepProject), Hotspots: deepProject})
	labels := drawnLabels(t, page)
	if len(labels) < 30 {
		t.Fatalf("only %d labels on the page; the fixture is not exercising the map", len(labels))
	}
	for i := range labels {
		for j := i + 1; j < len(labels); j++ {
			a, b := labels[i], labels[j]
			if area := overlap(a.ink, b.ink); area > 0.01 {
				t.Fatalf("%q %+v and %q %+v are drawn over each other (%.2f square units of ink)",
					a.text, a.ink, b.text, b.ink, area)
			}
		}
	}
}

// the map may drop a label it has no room for, but it may never drop a box: the
// fix for an unreadable label must not take a file off the map.
func TestDeepNestingStillGivesEveryFileABox(t *testing.T) {
	root := Tree(deepProject)
	Layout(root, canvasW, canvasH)
	seen := 0
	var walk func(n *Node)
	walk = func(n *Node) {
		if n.Leaf() {
			seen++
			if n.Rect.W <= 0 || n.Rect.H <= 0 {
				t.Fatalf("%s got no box: %+v", n.Path, n.Rect)
			}
			return
		}
		for _, c := range n.Children {
			walk(c)
		}
	}
	walk(root)
	if seen != len(deepProject) {
		t.Fatalf("laid out %d files, want %d", seen, len(deepProject))
	}
}

func TestHeatRunsColdToHotAndNeverReordersTwoFiles(t *testing.T) {
	if got := heat(0); got != "#eef2f6" {
		t.Fatalf("heat(0) = %s, want #eef2f6", got)
	}
	if got := heat(1); got != "#b02a1c" {
		t.Fatalf("heat(1) = %s, want #b02a1c", got)
	}
	// out of range scores clamp rather than producing a colour at all
	if heat(-1) != heat(0) || heat(9) != heat(1) {
		t.Fatal("heat does not clamp to the ends of the ramp")
	}
	// the ramp is monotone in the score, so a hotter file is never painted
	// cooler than a colder one.
	prev := 256
	for i := 0; i <= 100; i++ {
		var r, g, b int
		if _, err := fmt.Sscanf(heat(float64(i)/100), "#%02x%02x%02x", &r, &g, &b); err != nil {
			t.Fatal(err)
		}
		if r > prev {
			t.Fatalf("red channel rose at score %.2f", float64(i)/100)
		}
		prev = r
	}
}

func TestInkStaysReadableOnBothEndsOfTheRamp(t *testing.T) {
	if got := ink(heat(0)); got != "#1b2733" {
		t.Fatalf("ink on the cold end = %s, want dark", got)
	}
	if got := ink(heat(1)); got != "#ffffff" {
		t.Fatalf("ink on the hot end = %s, want white", got)
	}
}

func TestFitShortensToTheBox(t *testing.T) {
	if got := fit("session.ts", 200); got != "session.ts" {
		t.Fatalf("fit in a wide box = %q, want the whole name", got)
	}
	if got := fit("session.ts", 8); got != "" {
		t.Fatalf("fit in a sliver = %q, want no label", got)
	}
	got := fit("averylongfilename.ts", 50)
	if len(got) != 7 || !strings.HasSuffix(got, "..") {
		t.Fatalf("fit = %q, want 7 characters ending in ..", got)
	}
}

// a file name is whatever the filesystem allowed. none of it may reach the
// browser as markup.
func TestRenderEscapesFileNames(t *testing.T) {
	out := Render(Page{
		Project:  `<b>proj</b>`,
		Files:    1,
		Hotspots: []Item{{Path: `src/<script>alert("x")</script> & co.ts`, Complexity: 9, Score: 0.5}},
	})
	if strings.Contains(out, "<script") || strings.Contains(out, "alert(\"x\")") {
		t.Fatal("a file name reached the page as markup")
	}
	if !strings.Contains(out, "&lt;script&gt;") {
		t.Fatal("the escaped name is not on the page at all")
	}
	if strings.Contains(out, "<b>proj</b>") {
		t.Fatal("the project name reached the page as markup")
	}
}

// the page has to render off a disk with no network behind it.
func TestRenderIsSelfContained(t *testing.T) {
	out := Render(Page{Project: "p", Version: "0.1.0", Files: 2, Edges: 1,
		Hotspots: items("src/a.ts", "src/b.ts")})
	for _, bad := range []string{"<script", "<link", "<img", "src=", "@import", "url("} {
		if strings.Contains(out, bad) {
			t.Fatalf("the page pulls something in: %q", bad)
		}
	}
	if !strings.HasPrefix(out, "<!doctype html>") || !strings.HasSuffix(out, "</html>\n") {
		t.Fatal("the page is not a whole document")
	}
	if strings.Count(out, "<svg") != 1 || strings.Count(out, "</svg>") != 1 {
		t.Fatal("expected exactly one svg")
	}
}

// the numbers on the page are the ones the cli prints, in the same format.
func TestRenderCarriesTheCliNumbers(t *testing.T) {
	out := Render(Page{Project: "p", Files: 1, Edges: 0,
		Hotspots: []Item{{Path: "src/a.ts", Churn: 3, Complexity: 15, Score: 0.36}}})
	if !strings.Contains(out, "score 0.360, churn 3, complexity 15") {
		t.Fatal("the box tooltip does not carry the ranking numbers")
	}
	if !strings.Contains(out, "<td>0.360</td><td>3</td><td>15</td><td>src/a.ts</td>") {
		t.Fatal("the table row does not match the ranking")
	}
	if !strings.Contains(out, "1 files, 0 import edges, 1 ranked") {
		t.Fatal("the summary does not match the graph")
	}
}

func TestRenderEmptyProject(t *testing.T) {
	out := Render(Page{Project: "p"})
	if strings.Contains(out, "<svg") {
		t.Fatal("an empty project drew a map")
	}
	if !strings.Contains(out, "no source files here to map") {
		t.Fatal("an empty project says nothing about why the map is missing")
	}
}

// the same project has to produce the same bytes twice, or the page cannot be
// committed next to the code or diffed between two runs.
func TestRenderIsDeterministic(t *testing.T) {
	p := Page{Project: "p", Files: 6, Edges: 4, Hotspots: items(
		"src/a.ts", "src/b.ts", "src/deep/c.ts", "src/deep/d.ts", "e.ts", "f.ts")}
	if Render(p) != Render(p) {
		t.Fatal("two renders of one page differ")
	}
}
