// Package config reads warpmap.json, the optional per-project file that holds
// the two things every command used to hardcode: which directories to skip on
// the walk, and where "too risky" starts.
//
// It is optional on purpose. A project with no warpmap.json gets exactly the
// behaviour it got before this file existed - Defaults() is the old hardcoded
// list and the old hardcoded number, not a new set of opinions.
package config

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
)

// Config is the whole file. Both sections are optional; a missing one falls
// back to its default rather than to a zero value, because a `thresholds: {}`
// that silently meant blast=0 would fail every risk check in the repo.
type Config struct {
	Ignore     []string   `json:"ignore"`
	Thresholds Thresholds `json:"thresholds"`
}

// Thresholds are the numbers a command compares against. Kept as a named
// struct rather than loose fields so the Action added later gates on the same
// numbers a local run does, without re-deriving any of them.
type Thresholds struct {
	// Blast is how many dependents a file needs before changing it untested
	// counts as risky. `risk` compares with >, so 10 means "eleven or more".
	Blast int `json:"blast"`
}

// FileName is the name looked for in the analysed directory.
const FileName = "warpmap.json"

// builtinIgnore is what the walk skipped before warpmap.json existed. It stays
// in force even when a project supplies its own list - see Ignored.
var builtinIgnore = []string{"node_modules", ".git", "dist", "build"}

// Defaults is the configuration of a project that has no warpmap.json.
func Defaults() Config {
	ig := make([]string, len(builtinIgnore))
	copy(ig, builtinIgnore)
	return Config{Ignore: ig, Thresholds: Thresholds{Blast: 10}}
}

// Load reads dir/warpmap.json. A missing file is not an error: it is the
// normal case and yields Defaults(). A malformed one IS an error, because
// silently analysing a project under thresholds its author did not write is
// worse than refusing to start.
func Load(dir string) (Config, error) {
	b, err := os.ReadFile(filepath.Join(dir, FileName))
	if err != nil {
		if os.IsNotExist(err) {
			return Defaults(), nil
		}
		return Defaults(), err
	}

	// unmarshal onto the defaults, so a file that sets only one section keeps
	// the default for the other instead of zeroing it.
	c := Defaults()
	if err := json.Unmarshal(b, &c); err != nil {
		return Defaults(), err
	}

	// a file may add ignores; it may not drop the built-ins. skipping .git is
	// not a preference, and a project that lists "vendor" is asking for one
	// more directory to be skipped, not for node_modules to come back.
	c.Ignore = dedup(builtinIgnore, c.Ignore)
	if c.Thresholds.Blast < 0 {
		c.Thresholds.Blast = 0
	}
	return c, nil
}

func dedup(base, extra []string) []string {
	seen := map[string]bool{}
	out := make([]string, 0, len(base)+len(extra))
	for _, s := range append(append([]string{}, base...), extra...) {
		s = strings.TrimSpace(s)
		if s == "" || seen[s] {
			continue
		}
		seen[s] = true
		out = append(out, s)
	}
	return out
}

// Ignored reports whether a path relative to the project root should be
// skipped.
//
// A pattern is matched against BOTH the full relative path and the last
// segment, which is the only way to keep the old behaviour and still allow a
// targeted rule. "node_modules" has to keep matching at every depth, the way
// the hardcoded walk did; "src/generated" has to match only there. Matching
// both spellings gives each of those without a second syntax.
func (c Config) Ignored(relPath string) bool {
	relPath = filepath.ToSlash(relPath)
	base := relPath
	if i := strings.LastIndex(relPath, "/"); i >= 0 {
		base = relPath[i+1:]
	}
	for _, pat := range c.Ignore {
		pat = filepath.ToSlash(pat)
		if ok, err := filepath.Match(pat, relPath); err == nil && ok {
			return true
		}
		if ok, err := filepath.Match(pat, base); err == nil && ok {
			return true
		}
	}
	return false
}
