package graph

import (
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
)

type Edge struct {
	From string
	To   string
}

type Graph struct {
	Files []string
	Edges []Edge
}

// --- ts / js ---

// matches: import ... from "spec" | export ... from "spec" | require("spec") |
// import("spec") | import "spec"
var tsImportRe = regexp.MustCompile(
	`(?:import|export)[^'"]*?from\s*['"]([^'"]+)['"]` +
		`|(?:require|import)\(\s*['"]([^'"]+)['"]\s*\)` +
		`|(?:^|\n)\s*import\s*['"]([^'"]+)['"]`)

var tsExts = []string{".ts", ".tsx", ".js", ".jsx"}
var tsIndex = []string{"index.ts", "index.tsx", "index.js", "index.jsx"}

func extractTs(src string) []string {
	var specs []string
	for _, m := range tsImportRe.FindAllStringSubmatch(src, -1) {
		for _, g := range m[1:] {
			if g != "" {
				specs = append(specs, g)
			}
		}
	}
	return specs
}

func resolveTs(fromFile, spec string) string {
	if strings.HasPrefix(spec, ".") {
		return resolveOnDisk(filepath.Join(filepath.Dir(fromFile), spec))
	}
	if tc := findTsconfig(filepath.Dir(fromFile)); tc != nil {
		return resolveAlias(tc, spec)
	}
	return ""
}

// --- python ---

// a `from M import a, b` statement becomes one spec per name, "M\x00a", because
// each name may be a submodule of M (and so its own file) or an attribute of M.
// a plain `import M` is the spec "M".
var (
	pyFromRe    = regexp.MustCompile(`(?m)^[ \t]*from[ \t]+([\w.]+)[ \t]+import[ \t]*(\([^)]*\)|(?:\\\r?\n|[^\n#;])*)`)
	pyImportRe  = regexp.MustCompile(`(?m)^[ \t]*import[ \t]+((?:\\\r?\n|[^\n#;])+)`)
	pyCommentRe = regexp.MustCompile(`#[^\n]*`)
	pyNameRe    = regexp.MustCompile(`^[\w.]+$`)
)

func extractPy(src string) []string {
	var specs []string
	for _, m := range pyFromRe.FindAllStringSubmatch(src, -1) {
		names := pyNames(m[2])
		if len(names) == 0 {
			specs = append(specs, m[1])
		}
		for _, n := range names {
			specs = append(specs, m[1]+"\x00"+n)
		}
	}
	for _, m := range pyImportRe.FindAllStringSubmatch(src, -1) {
		specs = append(specs, pyNames(m[1])...)
	}
	return specs
}

// pyNames reads the names out of an import list, dropping comments, the
// parentheses and line continuations around it, and any `as` alias.
func pyNames(list string) []string {
	list = pyCommentRe.ReplaceAllString(list, "")
	list = strings.NewReplacer("(", " ", ")", " ", "\\", " ").Replace(list)
	var out []string
	for _, part := range strings.Split(list, ",") {
		if f := strings.Fields(part); len(f) > 0 && pyNameRe.MatchString(f[0]) {
			out = append(out, f[0])
		}
	}
	return out
}

func resolvePy(p *project, fromFile, spec string) string {
	mod, name, _ := strings.Cut(spec, "\x00")
	var to string
	if strings.HasPrefix(mod, ".") {
		to = pyLookup([]string{pyRelativeDir(fromFile, mod)}, strings.TrimLeft(mod, "."), name)
	} else {
		to = pyLookup(p.pyRootsFor(fromFile), mod, name)
	}
	// a package's __init__.py importing a name from its own package lands on
	// itself, and an edge to itself would count the file as imported.
	if to == fromFile {
		return ""
	}
	return to
}

// pyRelativeDir is the directory a relative module starts from: one leading dot
// is the importing file's own package, each further dot one level up.
func pyRelativeDir(fromFile, mod string) string {
	dir := filepath.Dir(fromFile)
	for i := 1; i < len(mod) && mod[i] == '.'; i++ {
		dir = filepath.Dir(dir)
	}
	return dir
}

// pyLookup finds the file for module mod (dotted, possibly empty) under the
// first root that has it, trying mod.name as a submodule before mod itself.
func pyLookup(roots []string, mod, name string) string {
	rel := filepath.FromSlash(strings.ReplaceAll(mod, ".", "/"))
	for _, root := range roots {
		base := filepath.Join(root, rel)
		if name != "" {
			if f := pyModuleFile(filepath.Join(base, name)); f != "" {
				return f
			}
		}
		if mod == "" {
			if f := filepath.Join(base, "__init__.py"); exists(f) {
				return f
			}
		} else if f := pyModuleFile(base); f != "" {
			return f
		}
	}
	return ""
}

func pyModuleFile(base string) string {
	for _, cand := range []string{base + ".py", filepath.Join(base, "__init__.py")} {
		if exists(cand) {
			return cand
		}
	}
	return ""
}

// project is what resolution knows about the whole file set rather than one
// file: for python, the directories an absolute import is looked up from.
type project struct {
	pyRoots []string
	isPkg   map[string]bool
}

// newProject derives python's source roots from the files themselves: the
// parent of every top-level package (a directory with __init__.py whose parent
// has none), then the directory common to every file and its src/. A name
// found under none of them is an installed or standard-library module and
// gets no edge.
func newProject(files []string) *project {
	p := &project{isPkg: map[string]bool{}}
	seen := map[string]bool{}
	var pkgRoots []string
	for _, f := range files {
		if filepath.Ext(f) != ".py" {
			continue
		}
		dir := filepath.Dir(f)
		if !p.pkg(dir) {
			continue
		}
		for p.pkg(dir) && filepath.Dir(dir) != dir {
			dir = filepath.Dir(dir)
		}
		if !seen[dir] {
			seen[dir] = true
			pkgRoots = append(pkgRoots, dir)
		}
	}
	sort.Strings(pkgRoots)
	p.pyRoots = pkgRoots
	if common, ok := commonDir(files); ok {
		for _, d := range []string{common, filepath.Join(common, "src")} {
			if !seen[d] && !p.pkg(d) && exists(d) {
				seen[d] = true
				p.pyRoots = append(p.pyRoots, d)
			}
		}
	}
	return p
}

func (p *project) pkg(dir string) bool {
	is, ok := p.isPkg[dir]
	if !ok {
		is = exists(filepath.Join(dir, "__init__.py"))
		p.isPkg[dir] = is
	}
	return is
}

// pyRootsFor puts a file's own directory first when that directory is not a
// package: python runs such a file as a script, with its directory on the path.
func (p *project) pyRootsFor(fromFile string) []string {
	dir := filepath.Dir(fromFile)
	if p.pkg(dir) {
		return p.pyRoots
	}
	roots := []string{dir}
	for _, r := range p.pyRoots {
		if r != dir {
			roots = append(roots, r)
		}
	}
	return roots
}

// commonDir is the deepest directory containing every file.
func commonDir(files []string) (string, bool) {
	if len(files) == 0 {
		return "", false
	}
	common := filepath.Dir(files[0])
	for _, f := range files[1:] {
		dir := filepath.Dir(f)
		for !within(dir, common) {
			parent := filepath.Dir(common)
			if parent == common {
				return "", false
			}
			common = parent
		}
	}
	return common, true
}

func within(dir, root string) bool {
	r, err := filepath.Rel(root, dir)
	return err == nil && r != ".." && !strings.HasPrefix(r, ".."+string(filepath.Separator))
}

// --- dispatch by extension ---

func extractImports(path, src string) []string {
	if l, ok := langFor(path); ok {
		return l.extract(src)
	}
	return nil
}

func resolve(p *project, fromFile, spec string) string {
	if l, ok := langFor(fromFile); ok {
		return l.resolve(p, fromFile, spec)
	}
	return ""
}

func exists(p string) bool {
	_, err := os.Stat(p)
	return err == nil
}

func hasExt(p string, exts []string) bool {
	e := filepath.Ext(p)
	for _, x := range exts {
		if e == x {
			return true
		}
	}
	return false
}

// Build resolves intra-project imports into a dependency graph.
func Build(files []string) Graph {
	g := Graph{Files: files}
	p := newProject(files)
	seen := map[string]bool{}
	for _, f := range files {
		src, err := os.ReadFile(f)
		if err != nil {
			continue
		}
		for _, spec := range extractImports(f, string(src)) {
			to := resolve(p, f, spec)
			if to == "" {
				continue
			}
			key := f + "\x00" + to
			if seen[key] {
				continue
			}
			seen[key] = true
			g.Edges = append(g.Edges, Edge{From: f, To: to})
		}
	}
	return g
}
