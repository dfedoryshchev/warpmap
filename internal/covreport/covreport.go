// Package covreport reads the coverage report a project's own test run wrote:
// which lines and functions of each file ran under the suite. It reads
// reports, it never runs a suite.
package covreport

import (
	"bufio"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"

	"github.com/dfedoryshchev/warpmap/internal/signal"
)

// Function is one function the report instruments: where it starts and how
// many times it was called.
type Function struct {
	Name string
	Line int
	Hits int
}

// File is what the report says about one file. Path is as the report wrote
// it; Lines maps each instrumented line, 1-based, to how many times it ran.
type File struct {
	Path      string
	Lines     map[int]int
	Functions []Function
}

// Ran reports whether any line or function of the file ran under the suite.
func (f File) Ran() bool {
	for _, n := range f.Lines {
		if n > 0 {
			return true
		}
	}
	for _, fn := range f.Functions {
		if fn.Hits > 0 {
			return true
		}
	}
	return false
}

// Report is a coverage report found in a project: its format, where it was
// read from (project-relative, slashed) and its files. Path is "" when the
// project has none.
type Report struct {
	Format string
	Path   string
	Files  []File
}

// Candidates are the places a report is looked for, project-relative, in
// order. It is read by path: the walk leaves coverage/ out, and a report is
// usually not tracked.
var Candidates = []string{"coverage/lcov.info", "lcov.info"}

// ParseLCOV reads an lcov tracefile. TN, FNF, FNH, LF, LH, the branch records
// and anything unknown are optional and not needed: lines and functions are
// counted from DA, FN and FNDA. A file that appears in more than one record
// has its hits added up.
func ParseLCOV(r io.Reader) ([]File, error) {
	var files []*File
	index := map[string]*File{}
	var cur *File
	fnLine := map[string]int{}
	fnHits := map[string]int{}
	var fnOrder []string
	flush := func() {
		if cur == nil {
			return
		}
		for _, name := range fnOrder {
			addFunction(cur, Function{Name: name, Line: fnLine[name], Hits: fnHits[name]})
		}
		cur, fnLine, fnHits, fnOrder = nil, map[string]int{}, map[string]int{}, nil
	}
	seen := func(name string) {
		if _, ok := fnLine[name]; ok {
			return
		}
		if _, ok := fnHits[name]; ok {
			return
		}
		fnOrder = append(fnOrder, name)
	}

	sc := bufio.NewScanner(r)
	sc.Buffer(make([]byte, 64*1024), 16*1024*1024)
	n := 0
	for sc.Scan() {
		n++
		line := strings.TrimSpace(sc.Text())
		if line == "" {
			continue
		}
		if line == "end_of_record" {
			flush()
			continue
		}
		key, val, _ := strings.Cut(line, ":")
		bad := func() error { return fmt.Errorf("line %d: %s", n, line) }
		switch key {
		case "TN", "VER":
			continue
		case "SF":
			flush()
			f, ok := index[val]
			if !ok {
				f = &File{Path: val, Lines: map[int]int{}}
				index[val] = f
				files = append(files, f)
			}
			cur = f
			continue
		}
		if cur == nil {
			return nil, fmt.Errorf("line %d: %s with no SF record", n, key)
		}
		switch key {
		case "DA":
			parts := strings.Split(val, ",")
			if len(parts) < 2 {
				return nil, bad()
			}
			ln, err1 := strconv.Atoi(parts[0])
			hits, err2 := strconv.Atoi(parts[1])
			if err1 != nil || err2 != nil || ln < 1 {
				return nil, bad()
			}
			cur.Lines[ln] += hits
		case "FN":
			// lcov 2 writes FN:<start>,<end>,<name>; earlier versions FN:<start>,<name>.
			start, name, ok := strings.Cut(val, ",")
			ln, err := strconv.Atoi(start)
			if !ok || err != nil {
				return nil, bad()
			}
			if end, rest, ok := strings.Cut(name, ","); ok {
				if _, err := strconv.Atoi(end); err == nil {
					name = rest
				}
			}
			seen(name)
			fnLine[name] = ln
		case "FNDA":
			hitsText, name, ok := strings.Cut(val, ",")
			hits, err := strconv.Atoi(hitsText)
			if !ok || err != nil {
				return nil, bad()
			}
			seen(name)
			fnHits[name] += hits
		}
	}
	if err := sc.Err(); err != nil {
		return nil, err
	}
	flush()
	out := make([]File, len(files))
	for i, f := range files {
		out[i] = *f
	}
	return out, nil
}

func addFunction(f *File, fn Function) {
	for i, have := range f.Functions {
		if have.Name == fn.Name {
			f.Functions[i].Hits += fn.Hits
			if have.Line == 0 {
				f.Functions[i].Line = fn.Line
			}
			return
		}
	}
	f.Functions = append(f.Functions, fn)
}

// Load reads the first report found under dir. No report is not an error: it
// returns a Report with no Path.
func Load(dir string) (Report, error) {
	for _, c := range Candidates {
		fh, err := os.Open(filepath.Join(dir, filepath.FromSlash(c)))
		if err != nil {
			continue
		}
		files, err := ParseLCOV(fh)
		fh.Close()
		if err != nil {
			return Report{}, fmt.Errorf("%s: %w", c, err)
		}
		return Report{Format: "lcov", Path: c, Files: files}, nil
	}
	return Report{}, nil
}

// rel turns a path as a report wrote it into one relative to dir, slashed, or
// "" when it lies outside dir.
func rel(absDir, p string) string {
	p = filepath.FromSlash(p)
	if filepath.IsAbs(p) {
		r, err := filepath.Rel(absDir, p)
		if err != nil {
			return ""
		}
		p = r
	}
	p = filepath.ToSlash(filepath.Clean(p))
	if p == ".." || strings.HasPrefix(p, "../") {
		return ""
	}
	return p
}

func absolute(dir string) string {
	if a, err := filepath.Abs(dir); err == nil {
		return a
	}
	return dir
}

const structural = "tested: a test imports the file (structural; %s)"

// Tested is what counts as tested for the walked files: the files the coverage
// report at from says ran under the suite, keyed as the walk spells them. ran
// is nil, and from "", when there is no report to go on, and the caller falls
// back to the import edge. The note says which, for stderr.
func Tested(dir string, files []string) (ran map[string]bool, from, note string) {
	r, err := Load(dir)
	if err != nil {
		return nil, "", fmt.Sprintf(structural, err)
	}
	if r.Path == "" {
		return nil, "", fmt.Sprintf(structural, "no coverage report at "+strings.Join(Candidates, " or "))
	}
	absDir := absolute(dir)
	walked := map[string]string{}
	for _, f := range files {
		walked[rel(absDir, absolute(f))] = f
	}
	ran = map[string]bool{}
	matched := 0
	for _, f := range r.Files {
		w, ok := walked[rel(absDir, f.Path)]
		if !ok {
			continue
		}
		matched++
		if f.Ran() {
			ran[w] = true
		}
	}
	if matched == 0 {
		return nil, "", fmt.Sprintf(structural, fmt.Sprintf("%s names %d files, none of them in the analysed tree", r.Path, len(r.Files)))
	}
	return ran, r.Path, "tested: a line ran under the suite, read from " + r.Path
}

// Adapter brings the report's files that ran no line at all into the report's
// findings.
type Adapter struct{}

func (Adapter) Tool() string { return "lcov" }

func (Adapter) Findings(dir string) ([]signal.Finding, error) {
	r, err := Load(dir)
	if err != nil || r.Path == "" {
		return nil, err
	}
	absDir := absolute(dir)
	instrumented := 0
	var never []string
	for _, f := range r.Files {
		p := rel(absDir, f.Path)
		if p == "" || len(f.Lines) == 0 && len(f.Functions) == 0 {
			continue
		}
		instrumented++
		if !f.Ran() {
			never = append(never, p)
		}
	}
	if len(never) == 0 {
		return nil, nil
	}
	sort.Strings(never)
	locs := make([]signal.Location, len(never))
	for i, p := range never {
		locs[i] = signal.Location{File: p}
	}
	return []signal.Finding{{
		RuleID:         "never-ran",
		Severity:       signal.Low,
		Message:        fmt.Sprintf("%d of the %d files with lines to run ran none of them under the suite", len(never), instrumented),
		Recommendation: "add a test that runs them before you change them",
		Locations:      locs,
	}}, nil
}
