package report

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"path/filepath"
	"sort"
	"strings"

	"github.com/dfedoryshchev/warpmap/internal/graph"
)

type Severity string

const (
	High Severity = "high"
	Med  Severity = "medium"
	Low  Severity = "low"
)

const namedFiles = 10

type Finding struct {
	ID             string   `json:"id"`
	Severity       Severity `json:"severity"`
	Kind           string   `json:"kind"`
	Detail         string   `json:"detail"`
	Recommendation string   `json:"recommendation"`
	Files          []string `json:"files"`
}

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
		subject := f.Files
		if f.Kind == "dead-code" {
			subject = nil
		}
		fs[i].ID = findingID(f.Kind, subject)
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
			Kind:           "god-module",
			Detail:         fmt.Sprintf("%s (in=%d out=%d)", f, m.FanIn, m.FanOut),
			Recommendation: "too central; a change here has a wide blast radius - split responsibilities",
			Files:          []string{f},
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
			Kind:           "cycle",
			Detail:         fmt.Sprintf("import cycle across %d files: %s", len(files), name(files, sep)),
			Recommendation: "break it - extract the shared piece into a third module",
			Files:          files,
		})
	}
	if files := relPaths(dir, graph.Orphans(dir, g)); len(files) > 0 {
		sort.Strings(files)
		fs = append(fs, Finding{
			Severity:       Low,
			Kind:           "dead-code",
			Detail:         fmt.Sprintf("%d files nothing imports: %s", len(files), name(files, ", ")),
			Recommendation: "confirm they are entry points, or delete them",
			Files:          files,
		})
	}
	return fs
}
