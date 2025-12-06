package metrics

import (
	"fmt"
	"os/exec"
	"strings"
)

// Churn counts how many commits touched each file within the window, keyed
// repo-relative exactly as git reports the path.
type Churn map[string]int

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
		churn[parts[2]]++
	}
	return churn, nil
}
