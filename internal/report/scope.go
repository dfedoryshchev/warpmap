package report

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/dfedoryshchev/warpmap/internal/gitcmd"
)

// Input is one analysed file and the sha256 of its bytes on disk.
type Input struct {
	File   string `json:"file"`
	SHA256 string `json:"sha256"`
}

// the checks a report runs, by the ids its findings and sections use.
var (
	graphRules   = []string{"cycle", "dead-code", "god-module", "hotspots"}
	historyRules = []string{"coupling", "held-by-one"}
)

const (
	noCommit = "none - not a git checkout, so only the input digest pins this run"
	noEgress = "no call made - `explain`, the one command that can make one, did not run"
	notBuilt = "nothing was built, installed or run: imports are read from the source text, and no test suite ran"
)

func head(dir string) (commit string, dirty bool) {
	out, err := gitcmd.Command(dir, "rev-parse", "--verify", "-q", "HEAD").Output()
	if err != nil {
		return "", false
	}
	status, err := gitcmd.Command(dir, "status", "--porcelain", "--", ".").Output()
	return strings.TrimSpace(string(out)), err == nil && len(strings.TrimSpace(string(status))) > 0
}

// inputs hashes every file the way sha256sum does and the list it prints, one
// "<hex>  <path>" line per file in byte order of path, so a client can check
// the digest with nothing but coreutils.
func inputs(dir string, files []string) ([]Input, string) {
	out := []Input{}
	for _, f := range files {
		b, err := os.ReadFile(f)
		if err != nil {
			continue
		}
		sum := sha256.Sum256(b)
		out = append(out, Input{File: relPath(dir, f), SHA256: hex.EncodeToString(sum[:])})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].File < out[j].File })
	var list strings.Builder
	for _, in := range out {
		list.WriteString(in.SHA256 + "  " + in.File + "\n")
	}
	sum := sha256.Sum256([]byte(list.String()))
	return out, hex.EncodeToString(sum[:])
}

func byExt(files []string) map[string]int {
	n := map[string]int{}
	for _, f := range files {
		n[filepath.Ext(f)]++
	}
	return n
}

// Coverage is complete when every check ran on every source file the walk
// found, partial when some check could not run or some files are in a language
// the graph does not parse, and unavailable when there was nothing to parse.
// The second value names the checks that could not run.
func (r Report) Coverage() (string, []string) {
	unsupported := []string{}
	switch {
	case r.Files == 0:
		unsupported = append(unsupported, graphRules...)
		unsupported = append(unsupported, historyRules...)
	case r.Window.Err != nil:
		unsupported = append(unsupported, historyRules...)
	}
	sort.Strings(unsupported)
	switch {
	case r.Files == 0:
		return "unavailable", unsupported
	case len(unsupported) > 0 || len(r.Unparsed) > 0:
		return "partial", unsupported
	}
	return "complete", unsupported
}

func projectName(dir string) string {
	abs, err := filepath.Abs(dir)
	if err != nil {
		return filepath.ToSlash(dir)
	}
	return filepath.Base(abs)
}

func countFiles(n int) string {
	if n == 1 {
		return "1 file"
	}
	return fmt.Sprintf("%d files", n)
}

// tally renders extension counts largest first: ".ts 40, .py 3".
func tally(n map[string]int) string {
	exts := make([]string, 0, len(n))
	for e := range n {
		exts = append(exts, e)
	}
	sort.Slice(exts, func(i, j int) bool {
		if n[exts[i]] != n[exts[j]] {
			return n[exts[i]] > n[exts[j]]
		}
		return exts[i] < exts[j]
	})
	parts := make([]string, len(exts))
	for i, e := range exts {
		parts[i] = fmt.Sprintf("%s %d", e, n[e])
	}
	return strings.Join(parts, ", ")
}

func (r Report) commitLine() string {
	switch {
	case r.Commit == "":
		return noCommit
	case r.Dirty:
		return "`" + r.Commit + "` with uncommitted changes"
	}
	return "`" + r.Commit + "`"
}

func (r Report) churnLine() string {
	if r.Window.Err != nil {
		return "no git history to read"
	}
	return r.Window.Span()
}

func (r Report) writeScope(b *strings.Builder) {
	b.WriteString("## scope\n\n")
	fmt.Fprintf(b, "- project: `%s`\n", projectName(r.Dir))
	fmt.Fprintf(b, "- commit: %s\n", r.commitLine())
	if r.Version != "" {
		fmt.Fprintf(b, "- warpmap: %s\n", r.Version)
	}
	fmt.Fprintf(b, "- inputs: %s, sha256 `%s`\n", countFiles(len(r.Inputs)), r.Digest)
	fmt.Fprintf(b, "- churn: %s\n", r.churnLine())
	fmt.Fprintf(b, "- network: %s\n", noEgress)
	status, unsupported := r.Coverage()
	fmt.Fprintf(b, "- coverage: %s\n", status)
	if len(unsupported) > 0 {
		fmt.Fprintf(b, "- not run: %s\n", strings.Join(unsupported, ", "))
	}
	b.WriteString("\n")

	b.WriteString("## limitations\n\n")
	fmt.Fprintf(b, "- parsed: %s", countFiles(r.Files))
	if len(r.Parsed) > 0 {
		fmt.Fprintf(b, " (%s)", tally(r.Parsed))
	}
	b.WriteString("\n")
	if len(r.Unparsed) == 0 {
		b.WriteString("- not parsed: no files in the other languages `hotspots` ranks\n")
	} else {
		fmt.Fprintf(b, "- not parsed: %s (%s) - no imports, findings or hotspots for these\n", countFiles(len(r.Unparsed)), tally(byExt(r.Unparsed)))
	}
	s := r.Specifiers
	verb := "are"
	if s.DroppedRelative == 1 {
		verb = "is"
	}
	fmt.Fprintf(b, "- import specifiers: %d resolved to a file in the project, %d did not; %d of those %s relative to the importing file\n",
		s.Kept, s.Dropped, s.DroppedRelative, verb)
	fmt.Fprintf(b, "- type-only edges: %d of %d", r.TypeOnlyEdges, r.Edges)
	if r.Edges > 0 {
		fmt.Fprintf(b, " (%.0f%%)", 100*float64(r.TypeOnlyEdges)/float64(r.Edges))
	}
	b.WriteString("\n")
	b.WriteString("- " + notBuilt + "\n\n")
}

type scope struct {
	Project     string   `json:"project"`
	Commit      string   `json:"commit"`
	Uncommitted bool     `json:"uncommitted_changes"`
	Warpmap     string   `json:"warpmap"`
	Inputs      inputSet `json:"inputs"`
	Churn       window   `json:"churn_window"`
	NetworkCall bool     `json:"network_call"`
	Narration   bool     `json:"narration"`
	Coverage    string   `json:"coverage"`
	Unsupported []string `json:"unsupported_rule_ids"`
}

type inputSet struct {
	Files  int     `json:"files"`
	SHA256 string  `json:"sha256"`
	Each   []Input `json:"each"`
}

type window struct {
	Months     int  `json:"months"`
	Commits    int  `json:"commits"`
	GitHistory bool `json:"git_history"`
}

type limitations struct {
	Parsed        map[string]int `json:"parsed"`
	NotParsed     map[string]int `json:"not_parsed"`
	Specifiers    specifiers     `json:"specifiers"`
	Edges         int            `json:"edges"`
	TypeOnlyEdges int            `json:"type_only_edges"`
}

type specifiers struct {
	Resolved        int `json:"resolved"`
	Dropped         int `json:"dropped"`
	DroppedRelative int `json:"dropped_relative"`
}

func (r Report) scopeJSON() scope {
	status, unsupported := r.Coverage()
	each := r.Inputs
	if each == nil {
		each = []Input{}
	}
	return scope{
		Project:     projectName(r.Dir),
		Commit:      r.Commit,
		Uncommitted: r.Dirty,
		Warpmap:     r.Version,
		Inputs:      inputSet{Files: len(r.Inputs), SHA256: r.Digest, Each: each},
		Churn:       window{Months: r.Window.Months, Commits: r.Window.Commits, GitHistory: r.Window.Err == nil},
		Coverage:    status,
		Unsupported: unsupported,
	}
}

func (r Report) limitationsJSON() limitations {
	parsed := r.Parsed
	if parsed == nil {
		parsed = map[string]int{}
	}
	return limitations{
		Parsed:        parsed,
		NotParsed:     byExt(r.Unparsed),
		Specifiers:    specifiers{Resolved: r.Specifiers.Kept, Dropped: r.Specifiers.Dropped, DroppedRelative: r.Specifiers.DroppedRelative},
		Edges:         r.Edges,
		TypeOnlyEdges: r.TypeOnlyEdges,
	}
}
