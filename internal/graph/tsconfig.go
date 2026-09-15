package graph

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
)

// tsconfig is the subset of tsconfig.json this resolver understands: the
// baseUrl/paths pair that remaps a bare specifier to an on-disk location.
// extends chains, project references and node_modules resolution are out of
// scope - a project relying on those resolves only its relative imports.
type tsconfig struct {
	baseURL string
	paths   map[string][]string
}

type tsconfigFile struct {
	CompilerOptions struct {
		BaseURL string              `json:"baseUrl"`
		Paths   map[string][]string `json:"paths"`
	} `json:"compilerOptions"`
}

// tsconfigDirCache maps a directory to the tsconfig covering it (found by
// walking upward), or nil when none exists up to the filesystem root. Every
// file under one project resolves through the same tsconfig.json, so caching
// by directory avoids re-walking and re-parsing per file.
var tsconfigDirCache = map[string]*tsconfig{}
var tsconfigFileCache = map[string]*tsconfig{}

// findTsconfig returns the tsconfig covering fromDir, or nil if none is
// found walking upward to the filesystem root.
func findTsconfig(fromDir string) *tsconfig {
	if tc, ok := tsconfigDirCache[fromDir]; ok {
		return tc
	}
	var visited []string
	dir := fromDir
	var found *tsconfig
	for {
		visited = append(visited, dir)
		candidate := filepath.Join(dir, "tsconfig.json")
		if tc, ok := tsconfigFileCache[candidate]; ok {
			found = tc
			break
		}
		if exists(candidate) {
			found = loadTsconfig(candidate)
			tsconfigFileCache[candidate] = found
			break
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			break
		}
		dir = parent
	}
	for _, v := range visited {
		tsconfigDirCache[v] = found
	}
	return found
}

// loadTsconfig reads path's compilerOptions.baseUrl and .paths. A tsconfig
// that fails to parse - most commonly because it carries the // comments tsc
// itself tolerates but encoding/json does not - is treated the same as no
// tsconfig: alias resolution is skipped, nothing else about the run changes.
func loadTsconfig(path string) *tsconfig {
	b, err := os.ReadFile(path)
	if err != nil {
		return nil
	}
	var f tsconfigFile
	if err := json.Unmarshal(b, &f); err != nil {
		return nil
	}
	if len(f.CompilerOptions.Paths) == 0 {
		return nil
	}
	base := f.CompilerOptions.BaseURL
	if base == "" {
		base = "."
	}
	return &tsconfig{
		baseURL: filepath.Join(filepath.Dir(path), base),
		paths:   f.CompilerOptions.Paths,
	}
}

// pathMatch is a paths pattern that matched a specifier, kept long enough to
// pick the best one when more than one pattern matches.
type pathMatch struct {
	pattern string
	prefix  string
	suffix  string
	hasStar bool
	targets []string
}

// resolveAlias maps spec through tc's paths table to a file on disk, sharing
// the extension/index rules relative resolution already uses. Returns "" when
// nothing in the table matches spec or none of a match's targets exist.
//
// tsc picks the longest matching pattern when more than one applies (its own
// paths entries have no other declared precedence, and encoding/json unmarshals
// a JSON object into a Go map anyway, which does not preserve source order) -
// an exact, star-free pattern always wins over a wildcard one.
func resolveAlias(tc *tsconfig, spec string) string {
	var best *pathMatch
	for pattern, targets := range tc.paths {
		prefix, suffix, hasStar := splitPattern(pattern)
		if hasStar {
			if !strings.HasPrefix(spec, prefix) || !strings.HasSuffix(spec, suffix) ||
				len(spec) < len(prefix)+len(suffix) {
				continue
			}
		} else if spec != pattern {
			continue
		}
		m := &pathMatch{pattern: pattern, prefix: prefix, suffix: suffix, hasStar: hasStar, targets: targets}
		if best == nil || betterMatch(m, best) {
			best = m
		}
	}
	if best == nil {
		return ""
	}
	star := ""
	if best.hasStar {
		star = spec[len(best.prefix) : len(spec)-len(best.suffix)]
	}
	for _, target := range best.targets {
		t := target
		if best.hasStar {
			t = strings.Replace(t, "*", star, 1)
		}
		if cand := resolveOnDisk(filepath.Join(tc.baseURL, t)); cand != "" {
			return cand
		}
	}
	return ""
}

func betterMatch(a, b *pathMatch) bool {
	if a.hasStar != b.hasStar {
		return !a.hasStar
	}
	if len(a.prefix) != len(b.prefix) {
		return len(a.prefix) > len(b.prefix)
	}
	return a.pattern < b.pattern
}

// splitPattern splits a tsconfig paths pattern on its wildcard - the only
// wildcard shape tsc's own paths matching supports.
func splitPattern(pattern string) (prefix, suffix string, hasStar bool) {
	i := strings.IndexByte(pattern, '*')
	if i < 0 {
		return pattern, "", false
	}
	return pattern[:i], pattern[i+1:], true
}

// resolveOnDisk applies the same file/extension/index rules resolveTs uses
// for a relative specifier, over an already-joined absolute base path.
func resolveOnDisk(base string) string {
	if hasExt(base, tsExts) && exists(base) {
		return base
	}
	for _, ext := range tsExts {
		if exists(base + ext) {
			return base + ext
		}
	}
	for _, idx := range tsIndex {
		if cand := filepath.Join(base, idx); exists(cand) {
			return cand
		}
	}
	return ""
}
