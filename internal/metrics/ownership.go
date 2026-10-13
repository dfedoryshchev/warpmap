package metrics

import (
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
)

// Owners returns the distinct authors who have touched a file, most commits first.
// a file only one person has ever touched is a bus-factor-1 knowledge risk.
// git is asked from the file's own directory, so the answer comes from the repo
// that holds the file - a nested repo or a submodule included, whose history the
// project's top repo does not have - and the pathspec is just the file's name,
// whatever the caller's working directory.
//
// one person is one author however many spellings they committed under: %aN and
// %aE apply the repo's .mailmap, then commits that share an email or a name
// (ignoring case) are the same person, named by the spelling they used most.
func Owners(file string) []string {
	out, err := exec.Command("git", "-C", filepath.Dir(file), "log", "--format=%aN%x00%aE", "--", filepath.Base(file)).Output()
	if err != nil {
		return nil
	}
	parent := map[string]string{}
	find := func(k string) string {
		for parent[k] != k {
			parent[k] = parent[parent[k]]
			k = parent[k]
		}
		return k
	}
	join := func(a, b string) {
		for _, k := range []string{a, b} {
			if _, ok := parent[k]; !ok {
				parent[k] = k
			}
		}
		parent[find(a)] = find(b)
	}

	type commit struct{ name, key string }
	var commits []commit
	for _, line := range strings.Split(strings.TrimSpace(string(out)), "\n") {
		name, email, _ := strings.Cut(line, "\x00")
		name, email = strings.TrimSpace(name), strings.ToLower(strings.TrimSpace(email))
		if name == "" && email == "" {
			continue
		}
		nameKey, emailKey := "n:"+strings.ToLower(name), "e:"+email
		switch {
		case name == "":
			name, nameKey = email, emailKey
		case email == "":
			emailKey = nameKey
		}
		join(nameKey, emailKey)
		commits = append(commits, commit{name, nameKey})
	}

	counts := map[string]int{}
	spellings := map[string]map[string]int{}
	var order []string
	for _, c := range commits {
		p := find(c.key)
		if counts[p] == 0 {
			order = append(order, p)
			spellings[p] = map[string]int{}
		}
		counts[p]++
		spellings[p][c.name]++
	}
	names := map[string]string{}
	for _, c := range commits {
		p := find(c.key)
		if cur, ok := names[p]; !ok || spellings[p][c.name] > spellings[p][cur] {
			names[p] = c.name
		}
	}
	sort.SliceStable(order, func(i, j int) bool { return counts[order[i]] > counts[order[j]] })
	owners := make([]string, len(order))
	for i, p := range order {
		owners[i] = names[p]
	}
	return owners
}
