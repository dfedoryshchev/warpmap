package metrics

import (
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
)

// Owners returns the distinct authors who have touched a file, most commits first.
// a file only one person has ever touched is a bus-factor-1 knowledge risk.
// file is a path the walk produced under repoDir; git reads its pathspec from
// inside repoDir, so a repoDir that is itself relative has to come off it.
func Owners(repoDir, file string) []string {
	if r, err := filepath.Rel(repoDir, file); err == nil {
		file = r
	}
	out, err := exec.Command("git", "-C", repoDir, "log", "--format=%an", "--", file).Output()
	if err != nil {
		return nil
	}
	counts := map[string]int{}
	var order []string
	for _, a := range strings.Split(strings.TrimSpace(string(out)), "\n") {
		if a == "" {
			continue
		}
		if counts[a] == 0 {
			order = append(order, a)
		}
		counts[a]++
	}
	sort.SliceStable(order, func(i, j int) bool { return counts[order[i]] > counts[order[j]] })
	return order
}
