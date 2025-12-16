package metrics

import (
	"fmt"
	"os/exec"
	"regexp"
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
