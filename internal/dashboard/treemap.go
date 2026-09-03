package dashboard

import (
	"math"
	"sort"
	"strings"
)

// Rect is a laid-out box in the map's own coordinate space; the SVG viewBox
// turns it into pixels, so the numbers here never depend on a screen.
type Rect struct{ X, Y, W, H float64 }

// Node is one box in the map: a directory with children, or a file leaf.
type Node struct {
	Name     string // the path segment, which is what the label shows
	Path     string // slashed, relative to the analysed directory
	Size     float64
	Item     Item // the ranked file; zero for a directory
	Children []*Node
	Rect     Rect
}

// Leaf reports whether the node is a file rather than a directory.
func (n *Node) Leaf() bool { return len(n.Children) == 0 }

// Tree groups the ranked files into the directory tree the map draws. A file's
// Size is its complexity score, which is what its area shows; a directory's is
// the sum of everything underneath it.
//
// A file with no code in it scores 0. It still gets a one-unit sliver rather
// than a zero-width box: a file too thin to read is a smaller lie than a file
// that is silently not on the map at all.
func Tree(items []Item) *Node {
	root := &Node{}
	dirs := map[string]*Node{"": root}
	for _, it := range items {
		if it.Path == "" {
			continue
		}
		parts := strings.Split(it.Path, "/")
		parent := root
		for i := 0; i < len(parts)-1; i++ {
			path := strings.Join(parts[:i+1], "/")
			d, ok := dirs[path]
			if !ok {
				d = &Node{Name: parts[i], Path: path}
				dirs[path] = d
				parent.Children = append(parent.Children, d)
			}
			parent = d
		}
		size := float64(it.Complexity)
		if size < 1 {
			size = 1
		}
		parent.Children = append(parent.Children, &Node{
			Name: parts[len(parts)-1],
			Path: it.Path,
			Size: size,
			Item: it,
		})
	}
	root.roll()
	root.order()
	return root
}

// roll sums each directory's children into its own size.
func (n *Node) roll() float64 {
	if n.Leaf() {
		return n.Size
	}
	sum := 0.0
	for _, c := range n.Children {
		sum += c.roll()
	}
	n.Size = sum
	return sum
}

// order puts the biggest box first at every level. squarify needs a descending
// order to produce square-ish rows at all, and the name tie-break keeps the
// same project rendering byte for byte the same page on every run.
func (n *Node) order() {
	sort.SliceStable(n.Children, func(i, j int) bool {
		a, b := n.Children[i], n.Children[j]
		if a.Size != b.Size {
			return a.Size > b.Size
		}
		return a.Name < b.Name
	})
	for _, c := range n.Children {
		c.order()
	}
}

const (
	// headerH is the strip a directory keeps for its own label before its
	// children are laid out below it.
	headerH = 15.0
	// pad separates a directory's frame from what it contains.
	pad = 2.0
)

// Layout assigns every node a rectangle inside a w by h canvas.
func Layout(root *Node, w, h float64) {
	root.Rect = Rect{0, 0, w, h}
	place(root)
}

func place(n *Node) {
	if n.Leaf() {
		return
	}
	inner := n.Rect
	if n.Path != "" { // the root is the canvas itself and has no label of its own
		inner = Rect{
			X: n.Rect.X + pad,
			Y: n.Rect.Y + headerH,
			W: n.Rect.W - 2*pad,
			H: n.Rect.H - headerH - pad,
		}
		if inner.W <= 0 || inner.H <= 0 {
			inner = n.Rect // no room for a frame; the children take the whole box
		}
	}
	squarify(n.Children, inner)
	for _, c := range n.Children {
		place(c)
	}
}

// squarify fills r with the nodes, in order, using the squarified treemap of
// Bruls, Huizing and van Wijk: grow a row along the shorter side while that
// keeps the boxes closer to square, close it when the next one would not, and
// recurse into what is left. Slice-and-dice would give the same areas as
// slivers nobody can compare by eye, which defeats the point of a treemap.
func squarify(nodes []*Node, r Rect) {
	total := 0.0
	for _, n := range nodes {
		total += n.Size
	}
	if total <= 0 || r.W <= 0 || r.H <= 0 {
		for _, n := range nodes {
			n.Rect = Rect{X: r.X, Y: r.Y}
		}
		return
	}
	scale := r.W * r.H / total

	for i := 0; i < len(nodes); {
		if r.W <= 0 || r.H <= 0 {
			for _, n := range nodes[i:] {
				n.Rect = Rect{X: r.X, Y: r.Y}
			}
			return
		}
		side := math.Min(r.W, r.H)
		j := i + 1
		best := worst(nodes[i:j], side, scale)
		for j < len(nodes) {
			next := worst(nodes[i:j+1], side, scale)
			if next > best {
				break
			}
			best = next
			j++
		}
		r = layoutRow(nodes[i:j], r, scale)
		i = j
	}
}

// worst is the least square-like aspect ratio the row would have if it were
// closed now. The row grows while this number falls.
func worst(row []*Node, side, scale float64) float64 {
	sum, smallest, largest := 0.0, math.Inf(1), 0.0
	for _, n := range row {
		a := n.Size * scale
		sum += a
		smallest = math.Min(smallest, a)
		largest = math.Max(largest, a)
	}
	if sum <= 0 || smallest <= 0 {
		return math.Inf(1)
	}
	side2, sum2 := side*side, sum*sum
	return math.Max(side2*largest/sum2, sum2/(side2*smallest))
}

// layoutRow lays one row along the shorter side of r and returns what is left
// of r for the rows after it.
func layoutRow(row []*Node, r Rect, scale float64) Rect {
	sum := 0.0
	for _, n := range row {
		sum += n.Size * scale
	}
	if r.W <= r.H { // a band across the top
		t := math.Min(sum/r.W, r.H)
		x := r.X
		for _, n := range row {
			w := 0.0
			if t > 0 {
				w = n.Size * scale / t
			}
			n.Rect = Rect{X: x, Y: r.Y, W: w, H: t}
			x += w
		}
		return Rect{X: r.X, Y: r.Y + t, W: r.W, H: r.H - t}
	}
	t := math.Min(sum/r.H, r.W) // a band down the left
	y := r.Y
	for _, n := range row {
		h := 0.0
		if t > 0 {
			h = n.Size * scale / t
		}
		n.Rect = Rect{X: r.X, Y: y, W: t, H: h}
		y += h
	}
	return Rect{X: r.X + t, Y: r.Y, W: r.W - t, H: r.H}
}
