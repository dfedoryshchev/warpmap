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

// Churn counts how many commits touched each file within the window, keyed
// repo-relative exactly as git reports the path.
type Churn map[string]int

// git --numstat prints a rename as "path/{old => new}/file" or "{old => new}".
// pull out the new path so the change counts against the file that exists now.
var renameRe = regexp.MustCompile("\\{[^}]* => ([^}]*)\\}")

func normalizeRename(p string) string {
	p = renameRe.ReplaceAllString(p, "$1")
	return strings.ReplaceAll(p, "//", "/")
}

// GitChurn walks the last windowMonths of history and counts changes per file.
func GitChurn(repoDir string, windowMonths int) (Churn, error) {
	since := fmt.Sprintf("%d months ago", windowMonths)
	out, err := exec.Command("git", "-C", repoDir, "log", "--since", since, "--numstat", "--format=").Output()
	if err != nil {
		return nil, err
	}
	churn := Churn{}
	for _, line := range strings.Split(string(out), "\n") {
		parts := strings.Split(line, "\t")
		if len(parts) != 3 {
			continue
		}
		churn[normalizeRename(parts[2])]++
	}
	return churn, nil
}

// ProjectChurn is GitChurn for a project whose files may sit in more than one
// repo: a folder of clones, or a repo holding nested repos or submodules. The
// repo at dir is read as GitChurn reads it; every other repo that holds one of
// files is asked about its own history, and its paths are keyed relative to
// dir, where the lookup expects them. The error is GitChurn's for dir, and only
// when no other repo answered either.
func ProjectChurn(dir string, files []string, windowMonths int) (Churn, error) {
	churn, err := GitChurn(dir, windowMonths)
	if churn == nil {
		churn = Churn{}
	}
	answered := err == nil
	for _, root := range nestedRepos(dir, files) {
		c, cerr := GitChurn(root, windowMonths)
		if cerr != nil {
			continue
		}
		answered = true
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
		return nil, err
	}
	return churn, nil
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
