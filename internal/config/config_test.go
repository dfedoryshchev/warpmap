package config

import (
	"os"
	"path/filepath"
	"testing"
)

// the whole point of the file being optional: a project without one is
// analysed exactly as it was before warpmap.json existed.
func TestMissingFileIsNotAnError(t *testing.T) {
	c, err := Load(t.TempDir())
	if err != nil {
		t.Fatalf("Load on a dir with no %s: %v", FileName, err)
	}
	if c.Thresholds.Blast != 10 {
		t.Fatalf("blast = %d, want the pre-config default 10", c.Thresholds.Blast)
	}
	for _, want := range []string{"node_modules", ".git", "dist", "build"} {
		if !c.Ignored(want) {
			t.Fatalf("default config does not ignore %q", want)
		}
	}
}

// a file that sets one section must not zero the other. this is the bug the
// unmarshal-onto-defaults exists to prevent: `{"ignore":["vendor"]}` read into
// a blank struct gives blast=0, and blast=0 makes every single file risky.
func TestPartialFileKeepsTheOtherDefault(t *testing.T) {
	dir := write(t, `{"ignore": ["vendor"]}`)
	c, err := Load(dir)
	if err != nil {
		t.Fatal(err)
	}
	if c.Thresholds.Blast != 10 {
		t.Fatalf("blast = %d after a file that only set ignore, want 10", c.Thresholds.Blast)
	}
	if !c.Ignored("vendor") {
		t.Fatal("the file's own ignore entry was dropped")
	}
}

// listing your own ignores adds to the built-ins, it does not replace them.
// a project asking to skip "vendor" is not asking for .git back.
func TestOwnIgnoresDoNotDropTheBuiltins(t *testing.T) {
	dir := write(t, `{"ignore": ["vendor"]}`)
	c, err := Load(dir)
	if err != nil {
		t.Fatal(err)
	}
	if !c.Ignored("node_modules") || !c.Ignored(".git") {
		t.Fatalf("built-in ignores lost: %v", c.Ignore)
	}
}

// the old walk skipped a directory by NAME at any depth. a pattern with no
// slash has to keep doing that, or every monorepo regresses the day it adds a
// config file.
func TestBareNameMatchesAtAnyDepth(t *testing.T) {
	c := Defaults()
	for _, p := range []string{"node_modules", "packages/a/node_modules", "a/b/c/dist"} {
		if !c.Ignored(p) {
			t.Fatalf("%q not ignored; a bare pattern must match at any depth", p)
		}
	}
}

// a pattern WITH a slash is anchored to where it was written, so it can name
// one generated directory without swallowing every directory of that name.
func TestPathPatternIsAnchored(t *testing.T) {
	dir := write(t, `{"ignore": ["src/generated"]}`)
	c, err := Load(dir)
	if err != nil {
		t.Fatal(err)
	}
	if !c.Ignored("src/generated") {
		t.Fatal("src/generated should be ignored")
	}
	if c.Ignored("lib/generated") {
		t.Fatal("lib/generated was ignored; a slashed pattern must not match elsewhere")
	}
}

// windows hands the walk backslashes. the config is written with forward ones,
// and a rule that only worked on one platform would be worse than no rule.
func TestBackslashPathStillMatches(t *testing.T) {
	c := Defaults()
	if !c.Ignored(filepath.Join("packages", "a", "node_modules")) {
		t.Fatal("a backslashed path did not match a forward-slash pattern")
	}
}

// refusing to start beats analysing under numbers nobody wrote. a typo in the
// config must not silently become the defaults.
func TestMalformedFileIsAnError(t *testing.T) {
	dir := write(t, `{"thresholds": {"blast": }`)
	if _, err := Load(dir); err == nil {
		t.Fatal("a malformed warpmap.json was accepted")
	}
}

// a negative threshold is meaningless rather than malformed; it is clamped
// instead of rejected, so a stray minus does not stop a CI run cold.
func TestNegativeThresholdIsClamped(t *testing.T) {
	dir := write(t, `{"thresholds": {"blast": -5}}`)
	c, err := Load(dir)
	if err != nil {
		t.Fatal(err)
	}
	if c.Thresholds.Blast != 0 {
		t.Fatalf("blast = %d, want it clamped to 0", c.Thresholds.Blast)
	}
}

func write(t *testing.T, body string) string {
	t.Helper()
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, FileName), []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	return dir
}
