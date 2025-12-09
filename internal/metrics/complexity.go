package metrics

import (
	"os"
	"regexp"
	"strings"
)

// a cheap structural proxy: non-blank lines plus branching keywords. not cyclomatic
// complexity, but good enough to rank files against each other.
var branchRe = regexp.MustCompile("\\b(if|for|while|case|catch)\\b|&&|\\|\\||\\?\\?")

type Complexity struct {
	LOC      int
	Branches int
	Score    int
}

func FileComplexity(path string) (Complexity, error) {
	src, err := os.ReadFile(path)
	if err != nil {
		return Complexity{}, err
	}
	loc := 0
	for _, line := range strings.Split(string(src), "\n") {
		if strings.TrimSpace(line) != "" {
			loc++
		}
	}
	branches := len(branchRe.FindAllString(string(src), -1))
	return Complexity{LOC: loc, Branches: branches, Score: loc + branches*3}, nil
}
