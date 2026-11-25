package report

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"path/filepath"
	"sort"
	"strings"

	"github.com/dfedoryshchev/warpmap/internal/graph"
	"github.com/dfedoryshchev/warpmap/internal/signal"
)

type (
	Severity = signal.Severity
	Finding  = signal.Finding
)

const (
	High = signal.High
	Med  = signal.Med
	Low  = signal.Low
)

const namedFiles = 10

func locations(files []string) []signal.Location {
	out := make([]signal.Location, len(files))
	for i, f := range files {
		out[i] = signal.Location{File: f}
	}
	return out
}

// Files lists the files a finding points at, in its order.
func Files(f Finding) []string {
	out := make([]string, len(f.Locations))
	for i, l := range f.Locations {
		out[i] = l.File
	}
	return out
}

// graphFindings is the adapter for the findings warpmap reads off its own
// import graph.
type graphFindings struct{ g graph.Graph }

func (graphFindings) Tool() string { return signal.Warpmap }

func (a graphFindings) Findings(dir string) ([]Finding, error) { return Findings(a.g, dir), nil }

func relPath(dir, f string) string {
	r, err := filepath.Rel(dir, f)
	if err != nil {
		return f
	}
	return filepath.ToSlash(r)
}

func relPaths(dir string, fs []string) []string {
	out := make([]string, len(fs))
	for i, f := range fs {
		out[i] = relPath(dir, f)
	}
	return out
}

func name(files []string, sep string) string {
	if len(files) <= namedFiles {
		return strings.Join(files, sep)
	}
	return fmt.Sprintf("%s, and %d more", strings.Join(files[:namedFiles], sep), len(files)-namedFiles)
}

// findingID is the one id scheme warpmap's own findings use: wm-<kind>-<hash>,
// the hash taken over the kind and the finding's subject, project-relative and
// in path order. A god module's subject is its file and a cycle's its files;
// the dead-code finding has none, being one per project, so deleting some of
// its files keeps its id.
func findingID(kind string, subject []string) string {
	s := append([]string{}, subject...)
	sort.Strings(s)
	sum := sha256.Sum256([]byte(kind + "\n" + strings.Join(s, "\n")))
	return "wm-" + kind + "-" + hex.EncodeToString(sum[:4])
}

// Findings turns the graph analyses into ranked, actionable findings.
func Findings(g graph.Graph, dir string) []Finding {
	fs := findings(g, dir)
	for i, f := range fs {
		subject := Files(f)
		if f.RuleID == "dead-code" {
			subject = nil
		}
		fs[i].ID = findingID(f.RuleID, subject)
		fs[i].SourceTool = signal.Warpmap
	}
	return fs
}

func findings(g graph.Graph, dir string) []Finding {
	var fs []Finding
	for _, m := range graph.GodModules(g) {
		if !m.IsGod() {
			break
		}
		f := relPath(dir, m.File)
		fs = append(fs, Finding{
			Severity:       High,
			RuleID:         "god-module",
			Message:        fmt.Sprintf("%s (in=%d out=%d)", f, m.FanIn, m.FanOut),
			Recommendation: "too central; a change here has a wide blast radius - split responsibilities",
			Locations:      locations([]string{f}),
		})
	}
	for _, c := range graph.Cycles(g) {
		files := relPaths(dir, c.Files)
		sep := ", "
		if c.Ring {
			sep = " -> "
		}
		fs = append(fs, Finding{
			Severity:       Med,
			RuleID:         "cycle",
			Message:        fmt.Sprintf("import cycle across %d files: %s", len(files), name(files, sep)),
			Recommendation: "break it - extract the shared piece into a third module",
			Locations:      locations(files),
		})
	}
	if files := relPaths(dir, graph.Orphans(dir, g)); len(files) > 0 {
		sort.Strings(files)
		fs = append(fs, Finding{
			Severity:       Low,
			RuleID:         "dead-code",
			Message:        fmt.Sprintf("%d files nothing imports: %s", len(files), name(files, ", ")),
			Recommendation: "confirm they are entry points, or delete them",
			Locations:      locations(files),
		})
	}
	return fs
}
