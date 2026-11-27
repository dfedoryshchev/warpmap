package metrics

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/dfedoryshchev/warpmap/internal/gitcmd"
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
	window := "churn: " + w.Span()
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

// Span is how much history the window holds: "12 commits in the last 6 months".
func (w Window) Span() string {
	return fmt.Sprintf("%s in the last %s", plural(w.Commits, "commit"), plural(w.Months, "month"))
}

func plural(n int, unit string) string {
	if n == 1 {
		return "1 " + unit
	}
	return fmt.Sprintf("%d %ss", n, unit)
}

// Commit is one commit in the window: its hash, its author after the repo's
// .mailmap, and the files it changed, keyed as Churn keys them.
type Commit struct {
	Hash  string
	Name  string
	Email string
	Files []string
}

// GitCommits reads the last windowMonths of history under dir, one Commit per
// commit that changed something there. dir may sit below the repo root: only its
// own changes count, keyed relative to it.
func GitCommits(dir string, windowMonths int) ([]Commit, error) {
	since := fmt.Sprintf("%d months ago", windowMonths)
	out, err := gitcmd.Command(dir, "log", "--since", since, "--no-merges", "--no-ext-diff", "--no-textconv",
		"--relative", "-z", "--numstat", "--format=%x00%H%x00%aN%x00%aE", "--", ".").Output()
	if err != nil {
		return nil, err
	}
	return parseLog(string(out)), nil
}

// parseLog reads `git log -z --numstat --format=%x00%H%x00%aN%x00%aE`. Every
// field ends in NUL, so an empty field starts a commit, the next three are its
// header, and each file is `added<TAB>deleted<TAB>path`, or for a rename an empty
// path followed by the old and the new name as fields of their own.
func parseLog(out string) []Commit {
	fields := strings.Split(out, "\x00")
	var commits []Commit
	for i := 0; i < len(fields); i++ {
		f := fields[i]
		if f == "" {
			if i+3 >= len(fields) {
				break
			}
			commits = append(commits, Commit{Hash: strings.TrimSpace(fields[i+1]), Name: fields[i+2], Email: fields[i+3]})
			i += 3
			if i+1 < len(fields) {
				// git ends the header with a newline before the first file, inside that file's field.
				fields[i+1] = strings.TrimPrefix(fields[i+1], "\n")
				if fields[i+1] == "" {
					i++
				}
			}
			continue
		}
		parts := strings.SplitN(f, "\t", 3)
		if len(parts) != 3 || len(commits) == 0 {
			continue
		}
		path := parts[2]
		if path == "" {
			if i+2 >= len(fields) {
				break
			}
			path = fields[i+2]
			i += 2
		}
		c := &commits[len(commits)-1]
		c.Files = append(c.Files, path)
	}
	return commits
}

// ChurnOf counts the changes each file got across commits.
func ChurnOf(commits []Commit) Churn {
	churn := Churn{}
	for _, c := range commits {
		for _, f := range c.Files {
			churn[f]++
		}
	}
	return churn
}

// GitChurn walks the last windowMonths of history under dir and counts changes
// per file, and the commits that made them. dir may sit below the repo root:
// only its own changes count, keyed relative to it.
func GitChurn(dir string, windowMonths int) (Churn, int, error) {
	commits, err := GitCommits(dir, windowMonths)
	if err != nil {
		return nil, 0, err
	}
	return ChurnOf(commits), len(commits), nil
}

// ProjectChurn is GitChurn for a project whose files may sit in more than one
// repo; ProjectCommits says how each repo is read.
func ProjectChurn(dir string, files []string, windowMonths int) (Churn, Window) {
	commits, w := ProjectCommits(dir, files, windowMonths)
	return ChurnOf(commits), w
}

// ProjectCommits is GitCommits for a project whose files may sit in more than
// one repo: a folder of clones, or a repo holding nested repos or submodules.
// The repo at dir is read as GitCommits reads it; every other repo that holds
// one of files is asked about its own history, and its paths are keyed relative
// to dir, where the lookups expect them. The window's error is GitCommits' for
// dir, and only when no other repo answered either.
func ProjectCommits(dir string, files []string, windowMonths int) ([]Commit, Window) {
	w := Window{Months: windowMonths}
	commits, err := GitCommits(dir, windowMonths)
	answered := err == nil
	for _, root := range nestedRepos(dir, files) {
		nested, cerr := GitCommits(root, windowMonths)
		if cerr != nil {
			continue
		}
		answered = true
		prefix, rerr := filepath.Rel(dir, root)
		if rerr != nil {
			w.Commits += len(nested)
			continue
		}
		prefix = filepath.ToSlash(prefix) + "/"
		for _, c := range nested {
			for i, f := range c.Files {
				c.Files[i] = prefix + f
			}
			commits = append(commits, c)
		}
	}
	w.Commits += len(commits)
	if !answered {
		w.Err = err
	}
	return commits, w
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
