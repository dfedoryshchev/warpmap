package metrics

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
)

// Churn counts how many commits touched each file within the window, keyed by
// the path relative to the directory git was asked about, with forward slashes.
type Churn map[string]int

// ThinWindow is the fewest commits a churn window needs before its counts are
// worth ranking on. Below it, one commit more or less reorders the table.
const ThinWindow = 20

// Window is what reading the history found: how far back it looked, how many
// commits touched the project in that time, and why it could not read any.
type Window struct {
	Months  int
	Commits int
	Err     error
}

// Note is the line that says what the churn column rests on. ranked is the
// ranking built from it, so the note can say when it fell back to complexity.
func (w Window) Note(ranked []Hotspot) string {
	if w.Err != nil {
		return "churn: no git history to read here - ranked by complexity alone"
	}
	commits := plural(w.Commits, "commit")
	window := fmt.Sprintf("churn: %s in the last %s", commits, plural(w.Months, "month"))
	churned := false
	for _, h := range ranked {
		if h.Churn > 0 {
			churned = true
			break
		}
	}
	switch {
	case !churned && w.Commits > 0:
		return window + ", none to a ranked file - ranked by complexity alone"
	case !churned:
		return window + " - ranked by complexity alone; --months widens the window"
	case w.Commits < ThinWindow:
		return fmt.Sprintf("%s - under %d, so churn differences are mostly noise; --months widens the window", window, ThinWindow)
	}
	return window
}

func plural(n int, unit string) string {
	if n == 1 {
		return "1 " + unit
	}
	return fmt.Sprintf("%d %ss", n, unit)
}

// git --numstat prints a rename as "path/{old => new}/file" or "{old => new}".
// pull out the new path so the change counts against the file that exists now.
var renameRe = regexp.MustCompile("\\{[^}]* => ([^}]*)\\}")

func normalizeRename(p string) string {
	p = renameRe.ReplaceAllString(p, "$1")
	return strings.ReplaceAll(p, "//", "/")
}

// GitChurn walks the last windowMonths of history under dir and counts changes
// per file, and the commits that made them. dir may sit below the repo root:
// only its own changes count, keyed relative to it.
func GitChurn(dir string, windowMonths int) (Churn, int, error) {
	since := fmt.Sprintf("%d months ago", windowMonths)
	out, err := exec.Command("git", "-C", dir, "log", "--since", since, "--no-merges",
		"--relative", "--numstat", "--format=%H", "--", ".").Output()
	if err != nil {
		return nil, 0, err
	}
	churn, commits := Churn{}, 0
	for _, line := range strings.Split(string(out), "\n") {
		parts := strings.Split(line, "\t")
		if len(parts) != 3 {
			if strings.TrimSpace(line) != "" {
				commits++
			}
			continue
		}
		churn[normalizeRename(parts[2])]++
	}
	return churn, commits, nil
}

// ProjectChurn is GitChurn for a project whose files may sit in more than one
// repo: a folder of clones, or a repo holding nested repos or submodules. The
// repo at dir is read as GitChurn reads it; every other repo that holds one of
// files is asked about its own history, and its paths are keyed relative to
// dir, where the lookup expects them. The churn is never nil; the window's
// error is GitChurn's for dir, and only when no other repo answered either.
func ProjectChurn(dir string, files []string, windowMonths int) (Churn, Window) {
	w := Window{Months: windowMonths}
	churn, commits, err := GitChurn(dir, windowMonths)
	if churn == nil {
		churn = Churn{}
	}
	w.Commits = commits
	answered := err == nil
	for _, root := range nestedRepos(dir, files) {
		c, n, cerr := GitChurn(root, windowMonths)
		if cerr != nil {
			continue
		}
		answered = true
		w.Commits += n
		prefix, rerr := filepath.Rel(dir, root)
		if rerr != nil {
			continue
		}
		prefix = filepath.ToSlash(prefix) + "/"
		for p, n := range c {
			churn[prefix+p] += n
		}
	}
	if !answered {
		w.Err = err
	}
	return churn, w
}

// nestedRepos finds the root of every repo below dir that holds one of files,
// by looking for the nearest .git above each file's directory.
func nestedRepos(dir string, files []string) []string {
	top := filepath.Clean(dir)
	rootOf := map[string]string{}
	var find func(d string) string
	find = func(d string) string {
		if r, ok := rootOf[d]; ok {
			return r
		}
		r := ""
		if d != top {
			if _, err := os.Lstat(filepath.Join(d, ".git")); err == nil {
				r = d
			} else if parent := filepath.Dir(d); parent != d {
				r = find(parent)
			}
		}
		rootOf[d] = r
		return r
	}
	seen := map[string]bool{}
	var roots []string
	for _, f := range files {
		if r := find(filepath.Dir(f)); r != "" && !seen[r] {
			seen[r] = true
			roots = append(roots, r)
		}
	}
	sort.Strings(roots)
	return roots
}
